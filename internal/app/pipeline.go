package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"craftstory/internal/dialogue"
	"craftstory/internal/distribution"
	"craftstory/internal/search"
	"craftstory/internal/speech"
	"craftstory/internal/video"
)

const defaultParallelism = 2

type Pipeline struct {
	service *Service
}

type GenerateResult struct {
	Title         string
	ScriptContent string
	OutputDir     string
	AudioPath     string
	VideoPath     string
	Duration      float64
}

type UploadRequest struct {
	VideoPath   string
	Title       string
	Description string
}

type generationContext struct {
	ctx            context.Context
	pipeline       *Pipeline
	session        *session
	voices         []speech.VoiceConfig
	voiceMap       map[string]speech.VoiceConfig
	isConversation bool
}

type audioResult struct {
	data     []byte
	timings  []speech.WordTiming
	duration float64
	script   string
}

func NewPipeline(service *Service) *Pipeline {
	return &Pipeline{service: service}
}

func (pipeline *Pipeline) Generate(ctx context.Context, topic string) (*GenerateResult, error) {
	generation := pipeline.newGenerationContext(ctx)

	slog.Info("Generating script...", "conversation", generation.isConversation)
	script, err := generation.generateScript(topic)
	if err != nil {
		return nil, err
	}

	title := generation.generateTitle(script, topic)
	if err := generation.session.finalize(title); err != nil {
		return nil, err
	}
	_ = os.WriteFile(generation.session.scriptPath(), []byte(script), 0644)

	slog.Info("Generating audio...", "length", len(script))
	audio, err := generation.generateAudio(script)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(generation.session.audioPath(), audio.data, 0644); err != nil {
		return nil, fmt.Errorf("save audio: %w", err)
	}

	slog.Info("Fetching images...")
	images := generation.fetchImages(script, audio.timings)

	slog.Info("Assembling video...")
	result, err := generation.assemble(audio, images)
	if err != nil {
		return nil, err
	}

	return &GenerateResult{
		Title:         title,
		ScriptContent: script,
		OutputDir:     generation.session.dir,
		AudioPath:     generation.session.audioPath(),
		VideoPath:     result.OutputPath,
		Duration:      result.Duration,
	}, nil
}

func (pipeline *Pipeline) newGenerationContext(ctx context.Context) *generationContext {
	config := pipeline.service.Config()
	voices := pipeline.voices()
	return &generationContext{
		ctx:            ctx,
		pipeline:       pipeline,
		session:        newSession(config.Video.OutputDir),
		voices:         voices,
		voiceMap:       buildVoiceMap(voices),
		isConversation: config.Content.ConversationMode && len(voices) >= 2,
	}
}

func (generation *generationContext) generateScript(topic string) (string, error) {
	llmClient := generation.pipeline.service.LLM()
	wordCount := generation.calculateWordCount()

	if generation.isConversation {
		names := generation.speakerNames()
		return llmClient.GenerateConversation(generation.ctx, topic, names, wordCount)
	}

	return llmClient.GenerateScript(generation.ctx, topic, wordCount)
}

func (generation *generationContext) calculateWordCount() int {
	config := generation.pipeline.service.Config()

	if config.Content.WordCount > 0 {
		return config.Content.WordCount
	}

	targetDuration := config.Content.TargetDuration
	if targetDuration <= 0 {
		targetDuration = config.Video.MaxDuration * 0.85
	}

	speed := config.ElevenLabs.Speed
	if speed <= 0 {
		speed = 1.0
	}

	wordsPerMinute := 150.0 * speed
	wordCount := int(targetDuration * wordsPerMinute / 60.0)

	if wordCount < 50 {
		wordCount = 50
	}
	if wordCount > 500 {
		wordCount = 500
	}

	return wordCount
}

func (generation *generationContext) speakerNames() []string {
	names := make([]string, len(generation.voices))
	for i, voice := range generation.voices {
		names[i] = voice.Name
	}
	return names
}

func (generation *generationContext) generateTitle(script, fallback string) string {
	title, err := generation.pipeline.service.LLM().GenerateTitle(generation.ctx, script)
	if err != nil {
		return fallback
	}
	return title
}

func (generation *generationContext) generateAudio(script string) (*audioResult, error) {
	if !generation.isConversation {
		return generation.generateSingleAudio(script)
	}
	return generation.generateConversationAudio(script)
}

func (generation *generationContext) generateSingleAudio(script string) (*audioResult, error) {
	result, err := generation.pipeline.service.TTS().GenerateSpeechWithTimings(generation.ctx, script)
	if err != nil {
		return nil, fmt.Errorf("generate speech: %w", err)
	}
	return &audioResult{
		data:     result.Audio,
		timings:  result.Timings,
		duration: audioDuration(result.Timings),
		script:   script,
	}, nil
}

func (generation *generationContext) generateConversationAudio(script string) (*audioResult, error) {
	parsed := dialogue.Parse(script)
	if parsed.IsEmpty() {
		return generation.generateSingleAudio(script)
	}

	segments, err := generation.generateSpeechSegments(parsed)
	if err != nil {
		return nil, err
	}

	stitched, err := video.NewAudioStitcher(generation.pipeline.service.Config().Video.OutputDir).Stitch(generation.ctx, segments)
	if err != nil {
		return nil, fmt.Errorf("stitch audio: %w", err)
	}

	return &audioResult{
		data:     stitched.Data,
		timings:  stitched.Timings,
		duration: stitched.Duration,
		script:   parsed.FullText(),
	}, nil
}

func (generation *generationContext) generateSpeechSegments(parsed *dialogue.Script) ([]video.AudioSegment, error) {
	segments := make([]video.AudioSegment, len(parsed.Lines))
	defaultVoice := generation.voices[0]

	type lineJob struct {
		index int
		line  dialogue.Line
		voice speech.VoiceConfig
	}

	jobs := make([]lineJob, len(parsed.Lines))
	for i, line := range parsed.Lines {
		voice, ok := generation.voiceMap[line.Speaker]
		if !ok {
			slog.Warn("unknown speaker, using default", "speaker", line.Speaker)
			voice = defaultVoice
		}
		jobs[i] = lineJob{index: i, line: line, voice: voice}
	}

	type result struct {
		index   int
		segment video.AudioSegment
		err     error
	}

	results := make(chan result, len(jobs))

	semaphore := make(chan struct{}, defaultParallelism)

	for _, job := range jobs {
		go func(j lineJob) {
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			slog.Info("Generating speech", "line", j.index+1, "total", len(parsed.Lines), "speaker", j.line.Speaker)
			speechResult, err := generation.pipeline.service.TTS().GenerateSpeechWithVoice(generation.ctx, j.line.Text, j.voice)
			if err != nil {
				results <- result{index: j.index, err: fmt.Errorf("generate speech for line %d: %w", j.index+1, err)}
				return
			}

			results <- result{
				index: j.index,
				segment: video.AudioSegment{
					Audio:   speechResult.Audio,
					Timings: speechResult.Timings,
					Speaker: j.line.Speaker,
				},
			}
		}(job)
	}

	for range jobs {
		r := <-results
		if r.err != nil {
			return nil, r.err
		}
		segments[r.index] = r.segment
	}

	return segments, nil
}

func (generation *generationContext) fetchImages(script string, timings []speech.WordTiming) []video.ImageOverlay {
	fetcher := generation.pipeline.service.Fetcher()
	if fetcher == nil {
		slog.Warn("Image fetcher not configured (missing GOOGLE_SEARCH_API_KEY or GOOGLE_SEARCH_ENGINE_ID)")
		return nil
	}

	config := generation.pipeline.service.Config()
	count := config.Visuals.Count
	if count <= 0 {
		count = 5
	}

	slog.Info("Generating visual cues from script...", "count", count)
	cues, err := generation.pipeline.service.LLM().GenerateVisuals(generation.ctx, script, count)
	if err != nil {
		slog.Warn("Failed to generate visuals", "error", err)
		return nil
	}
	slog.Info("Generated visual cues", "count", len(cues))
	for i, cue := range cues {
		slog.Info("Visual cue", "index", i, "keyword", cue.Keyword, "query", cue.SearchQuery)
	}

	searchCues := make([]search.VisualCue, len(cues))
	for i, cue := range cues {
		searchCues[i] = search.VisualCue{
			Keyword:     cue.Keyword,
			SearchQuery: cue.SearchQuery,
			Type:        cue.Type,
		}
	}

	slog.Info("Fetching images from Google...", "timings_count", len(timings))
	return fetcher.Fetch(generation.ctx, search.FetchRequest{
		Script:   script,
		Visuals:  searchCues,
		Timings:  timings,
		ImageDir: generation.session.dir,
	})
}

func (generation *generationContext) assemble(audio *audioResult, images []video.ImageOverlay) (*video.AssembleResult, error) {
	config := generation.pipeline.service.Config()
	if config.Video.MaxDuration > 0 && audio.duration > config.Video.MaxDuration {
		return nil, fmt.Errorf("audio duration %.1fs exceeds limit of %.0fs", audio.duration, config.Video.MaxDuration)
	}

	speakerColors := buildSpeakerColors(generation.voiceMap)

	return generation.pipeline.service.Assembler().Assemble(generation.ctx, video.AssembleRequest{
		AudioPath:     generation.session.audioPath(),
		AudioDuration: audio.duration,
		Script:        audio.script,
		OutputPath:    generation.session.videoPath(),
		WordTimings:   audio.timings,
		ImageOverlays: images,
		SpeakerColors: speakerColors,
	})
}

func (pipeline *Pipeline) voices() []speech.VoiceConfig {
	cfg := pipeline.service.Config()
	var result []speech.VoiceConfig

	if cfg.ElevenLabs.HostVoice.ID != "" {
		result = append(result, speech.VoiceConfig{
			ID:            cfg.ElevenLabs.HostVoice.ID,
			Name:          cfg.ElevenLabs.HostVoice.Name,
			SubtitleColor: cfg.ElevenLabs.HostVoice.SubtitleColor,
		})
	}

	if cfg.ElevenLabs.GuestVoice.ID != "" {
		result = append(result, speech.VoiceConfig{
			ID:            cfg.ElevenLabs.GuestVoice.ID,
			Name:          cfg.ElevenLabs.GuestVoice.Name,
			SubtitleColor: cfg.ElevenLabs.GuestVoice.SubtitleColor,
		})
	}

	return result
}

func (pipeline *Pipeline) GenerateFromReddit(ctx context.Context) (*GenerateResult, error) {
	topic, err := pipeline.fetchRedditTopic(ctx)
	if err != nil {
		return nil, err
	}
	return pipeline.Generate(ctx, topic)
}

func (pipeline *Pipeline) fetchRedditTopic(ctx context.Context) (string, error) {
	cfg := pipeline.service.Config()
	redditCfg := cfg.Reddit

	subreddits := redditCfg.Subreddits
	if len(subreddits) == 0 {
		subreddits = []string{"cscareerquestions", "learnprogramming"}
	}

	subreddit := subreddits[randomInt(len(subreddits))]
	sort := redditCfg.Sort
	if sort == "" {
		sort = "hot"
	}
	postLimit := redditCfg.PostLimit
	if postLimit <= 0 {
		postLimit = 10
	}

	slog.Info("Fetching Reddit posts", "subreddit", subreddit, "sort", sort)
	posts, err := pipeline.service.Reddit().GetSubredditPosts(ctx, subreddit, sort, postLimit)
	if err != nil {
		return "", fmt.Errorf("fetch reddit posts: %w", err)
	}
	if len(posts) == 0 {
		return "", fmt.Errorf("no posts found in subreddit: %s", subreddit)
	}

	post := posts[randomInt(len(posts))]
	slog.Info("Selected post", "title", post.Title)

	return post.Title, nil
}

func (pipeline *Pipeline) Upload(ctx context.Context, request UploadRequest) (*distribution.UploadResponse, error) {
	config := pipeline.service.Config()
	response, err := pipeline.service.Uploader().Upload(ctx, distribution.UploadRequest{
		FilePath:    request.VideoPath,
		Title:       request.Title,
		Description: request.Description,
		Tags:        config.YouTube.DefaultTags,
		Privacy:     config.YouTube.PrivacyStatus,
	})
	if err != nil {
		return nil, fmt.Errorf("upload video: %w", err)
	}
	return response, nil
}
