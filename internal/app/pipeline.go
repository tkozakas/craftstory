package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"craftstory/internal/dialogue"
	"craftstory/internal/llm"
	"craftstory/internal/tts"
	"craftstory/internal/uploader"
	"craftstory/internal/video"
	"craftstory/internal/visuals"
	"craftstory/pkg/config"
)

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
	voices         []tts.VoiceConfig
	voiceMap       map[string]tts.VoiceConfig
	isConversation bool
}

type audioResult struct {
	data         []byte
	timings      []tts.WordTiming
	charOverlays []video.CharacterOverlay
	duration     float64
	script       string
}

func NewPipeline(service *Service) *Pipeline {
	return &Pipeline{service: service}
}

func (pipeline *Pipeline) Generate(ctx context.Context, topic string) (*GenerateResult, error) {
	generation := pipeline.newGenerationContext(ctx)

	slog.Info("Generating script...", "conversation", generation.isConversation)
	script, cues, err := generation.generateScript(topic)
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
	images := generation.fetchImages(script, cues, audio.timings)

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

func (generation *generationContext) generateScript(topic string) (string, []llm.VisualCue, error) {
	config := generation.pipeline.service.Config()
	llmClient := generation.pipeline.service.LLM()

	if generation.isConversation {
		names := generation.speakerNames()
		script, err := llmClient.GenerateConversation(generation.ctx, topic, names, config.Content.ScriptLength, config.Content.HookDuration)
		if err != nil {
			return "", nil, fmt.Errorf("generate conversation: %w", err)
		}
		return script, nil, nil
	}

	if !config.Visuals.Enabled || generation.pipeline.service.Fetcher() == nil {
		script, err := llmClient.GenerateScript(generation.ctx, topic, config.Content.ScriptLength, config.Content.HookDuration)
		return script, nil, err
	}

	result, err := llmClient.GenerateScriptWithVisuals(generation.ctx, topic, config.Content.ScriptLength, config.Content.HookDuration)
	if err != nil {
		return "", nil, fmt.Errorf("generate script: %w", err)
	}
	return result.Script, result.Visuals, nil
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
		data:         stitched.Data,
		timings:      stitched.Timings,
		charOverlays: buildCharacterOverlays(stitched.Segments, generation.voiceMap),
		duration:     stitched.Duration,
		script:       parsed.FullText(),
	}, nil
}

func (generation *generationContext) generateSpeechSegments(parsed *dialogue.Script) ([]video.AudioSegment, error) {
	segments := make([]video.AudioSegment, len(parsed.Lines))
	defaultVoice := generation.voices[0]

	for i, line := range parsed.Lines {
		voice, ok := generation.voiceMap[line.Speaker]
		if !ok {
			slog.Warn("unknown speaker, using default", "speaker", line.Speaker)
			voice = defaultVoice
		}

		slog.Info("Generating speech", "line", i+1, "total", len(parsed.Lines), "speaker", line.Speaker)
		result, err := generation.pipeline.service.TTS().GenerateSpeechWithVoice(generation.ctx, line.Text, voice)
		if err != nil {
			return nil, fmt.Errorf("generate speech for line %d: %w", i+1, err)
		}

		segments[i] = video.AudioSegment{
			Audio:   result.Audio,
			Timings: result.Timings,
			Speaker: line.Speaker,
		}
	}
	return segments, nil
}

func (generation *generationContext) fetchImages(script string, cues []llm.VisualCue, timings []tts.WordTiming) []video.ImageOverlay {
	fetcher := generation.pipeline.service.Fetcher()
	if fetcher == nil {
		return nil
	}
	if len(cues) > 0 {
		return fetcher.Fetch(generation.ctx, visuals.FetchRequest{
			Script:   script,
			Visuals:  cues,
			Timings:  timings,
			ImageDir: generation.session.dir,
		})
	}
	return fetcher.FetchForConversation(generation.ctx, script, timings, generation.session.dir)
}

func (generation *generationContext) assemble(audio *audioResult, images []video.ImageOverlay) (*video.AssembleResult, error) {
	config := generation.pipeline.service.Config()
	if config.Video.MaxDuration > 0 && audio.duration > config.Video.MaxDuration {
		return nil, fmt.Errorf("audio duration %.1fs exceeds limit of %.0fs", audio.duration, config.Video.MaxDuration)
	}

	return generation.pipeline.service.Assembler().Assemble(generation.ctx, video.AssembleRequest{
		AudioPath:         generation.session.audioPath(),
		AudioDuration:     audio.duration,
		Script:            audio.script,
		OutputPath:        generation.session.videoPath(),
		WordTimings:       audio.timings,
		ImageOverlays:     images,
		CharacterOverlays: audio.charOverlays,
	})
}

func (pipeline *Pipeline) voices() []tts.VoiceConfig {
	cfg := pipeline.service.Config()
	var result []tts.VoiceConfig

	host := cfg.GetHost()
	if host != nil {
		result = append(result, tts.VoiceConfig{
			ID:     host.VoiceID,
			Name:   host.Name,
			Avatar: host.ImagePath,
		})
	}

	guest := cfg.GetGuest()
	if guest != nil {
		result = append(result, tts.VoiceConfig{
			ID:     guest.VoiceID,
			Name:   guest.Name,
			Avatar: guest.ImagePath,
		})
	}

	if len(result) > 0 {
		return result
	}

	var voices []config.Voice
	if cfg.FishAudio.Enabled {
		voices = cfg.FishAudio.Voices
	} else {
		voices = cfg.ElevenLabs.Voices
	}
	for _, voice := range voices {
		result = append(result, tts.VoiceConfig{
			ID:         voice.ID,
			Name:       voice.Name,
			Avatar:     voice.Avatar,
			Stability:  voice.Stability,
			Similarity: voice.Similarity,
		})
	}
	return result
}

func (pipeline *Pipeline) GenerateFromReddit(ctx context.Context) (*GenerateResult, error) {
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
	commentLimit := redditCfg.CommentLimit
	if commentLimit <= 0 {
		commentLimit = 15
	}

	slog.Info("Fetching Reddit posts", "subreddit", subreddit, "sort", sort)
	posts, err := pipeline.service.Reddit().GetSubredditPosts(ctx, subreddit, sort, postLimit)
	if err != nil {
		return nil, fmt.Errorf("fetch reddit posts: %w", err)
	}
	if len(posts) == 0 {
		return nil, fmt.Errorf("no posts found in subreddit: %s", subreddit)
	}

	post := posts[randomInt(len(posts))]
	slog.Info("Selected post", "title", post.Title)

	comments, err := pipeline.service.Reddit().GetPostComments(ctx, post.Permalink, commentLimit)
	if err != nil {
		slog.Warn("Failed to fetch comments", "error", err)
	}

	var commentTexts []string
	for _, c := range comments {
		if len(c.Body) > 500 {
			commentTexts = append(commentTexts, c.Body[:500]+"...")
		} else {
			commentTexts = append(commentTexts, c.Body)
		}
	}

	thread := llm.RedditThread{
		Title:    post.Title,
		Post:     post.Selftext,
		Comments: commentTexts,
	}

	return pipeline.generateFromThread(ctx, thread)
}

func (pipeline *Pipeline) generateFromThread(ctx context.Context, thread llm.RedditThread) (*GenerateResult, error) {
	generation := pipeline.newGenerationContext(ctx)
	cfg := pipeline.service.Config()

	slog.Info("Generating script from Reddit thread...", "title", thread.Title)
	names := generation.speakerNames()
	script, err := pipeline.service.LLM().GenerateRedditConversation(ctx, thread, names, cfg.Content.ScriptLength, cfg.Content.HookDuration)
	if err != nil {
		return nil, fmt.Errorf("generate reddit conversation: %w", err)
	}

	title := generation.generateTitle(script, thread.Title)
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
	images := generation.fetchImages(script, nil, audio.timings)

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

func (pipeline *Pipeline) Upload(ctx context.Context, request UploadRequest) (*uploader.UploadResponse, error) {
	config := pipeline.service.Config()
	response, err := pipeline.service.Uploader().Upload(ctx, uploader.UploadRequest{
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
