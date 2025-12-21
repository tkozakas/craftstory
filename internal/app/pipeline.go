package app

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"craftstory/internal/deepseek"
	"craftstory/internal/dialogue"
	"craftstory/internal/elevenlabs"
	"craftstory/internal/uploader"
	"craftstory/internal/video"
)

type Pipeline struct {
	svc *Service
}

type GenerateResult struct {
	Title         string
	ScriptContent string
	AudioPath     string
	VideoPath     string
	Duration      float64
}

func NewPipeline(svc *Service) *Pipeline {
	return &Pipeline{svc: svc}
}

func (p *Pipeline) Generate(ctx context.Context, topic string) (*GenerateResult, error) {
	cfg := p.svc.Config()

	slog.Info("pipeline config",
		"conversation_mode", cfg.Content.ConversationMode,
		"voices_count", len(cfg.ElevenLabs.Voices),
		"visuals_enabled", cfg.Visuals.Enabled,
		"image_search_available", p.svc.ImageSearch() != nil,
	)

	if cfg.Content.ConversationMode && len(cfg.ElevenLabs.Voices) >= 2 {
		return p.generateConversation(ctx, topic)
	}

	return p.generateSingleVoice(ctx, topic)
}

func (p *Pipeline) generateSingleVoice(ctx context.Context, topic string) (*GenerateResult, error) {
	cfg := p.svc.Config()

	var script string
	var visuals []deepseek.VisualCue

	slog.Info("generateSingleVoice starting",
		"visuals_enabled", cfg.Visuals.Enabled,
		"image_search_available", p.svc.ImageSearch() != nil,
	)

	if cfg.Visuals.Enabled && p.svc.ImageSearch() != nil {
		slog.Info("generating script with visuals", "topic", topic)
		result, err := p.svc.DeepSeek().GenerateScriptWithVisuals(ctx, topic, cfg.Content.ScriptLength, cfg.Content.HookDuration)
		if err != nil {
			return nil, fmt.Errorf("failed to generate script: %w", err)
		}
		script = result.Script
		visuals = result.Visuals
		slog.Info("generated script with visual cues",
			"script_length", len(script),
			"visuals_count", len(visuals),
			"visuals", visuals,
		)
	} else {
		slog.Info("generating script without visuals",
			"topic", topic,
			"reason_visuals_disabled", !cfg.Visuals.Enabled,
			"reason_no_image_search", p.svc.ImageSearch() == nil,
		)
		var err error
		script, err = p.svc.DeepSeek().GenerateScript(ctx, topic, cfg.Content.ScriptLength, cfg.Content.HookDuration)
		if err != nil {
			return nil, fmt.Errorf("failed to generate script: %w", err)
		}
	}

	slog.Info("generating speech", "script_length", len(script))
	speechResult, err := p.svc.ElevenLabs().GenerateSpeechWithTimings(ctx, script)
	if err != nil {
		return nil, fmt.Errorf("failed to generate speech: %w", err)
	}

	audioFilename := fmt.Sprintf("audio_%d.mp3", time.Now().Unix())
	audioPath, err := p.svc.Storage().SaveAudio(speechResult.Audio, audioFilename)
	if err != nil {
		return nil, fmt.Errorf("failed to save audio: %w", err)
	}
	slog.Info("saved audio", "path", audioPath)

	audioDuration := getAudioDuration(speechResult.Timings)

	var imageOverlays []video.ImageOverlay
	if len(visuals) > 0 {
		imageOverlays = p.fetchVisualImages(ctx, visuals, speechResult.Timings)
	}

	assembleReq := video.AssembleRequest{
		AudioPath:     audioPath,
		AudioDuration: audioDuration,
		Script:        script,
		ScriptID:      time.Now().Unix(),
		WordTimings:   speechResult.Timings,
		ImageOverlays: imageOverlays,
	}

	slog.Info("assembling video", "duration", audioDuration, "overlays", len(imageOverlays))
	result, err := p.svc.Assembler().Assemble(ctx, assembleReq)
	if err != nil {
		return nil, fmt.Errorf("failed to assemble video: %w", err)
	}
	slog.Info("video assembled", "path", result.OutputPath)

	title := p.generateTitle(ctx, script, topic)

	return &GenerateResult{
		Title:         title,
		ScriptContent: script,
		AudioPath:     audioPath,
		VideoPath:     result.OutputPath,
		Duration:      result.Duration,
	}, nil
}

func (p *Pipeline) fetchVisualImages(ctx context.Context, visuals []deepseek.VisualCue, timings []elevenlabs.WordTiming) []video.ImageOverlay {
	cfg := p.svc.Config()
	overlays := make([]video.ImageOverlay, 0, len(visuals))

	slog.Info("fetching visual images",
		"visual_cues", len(visuals),
		"word_timings", len(timings),
		"image_search_client", p.svc.ImageSearch() != nil,
	)

	if p.svc.ImageSearch() == nil {
		slog.Warn("image search client is nil, skipping visuals")
		return overlays
	}

	for i, cue := range visuals {
		slog.Info("processing visual cue",
			"index", i,
			"query", cue.SearchQuery,
			"word_index", cue.WordIndex,
		)

		if cue.WordIndex >= len(timings) {
			slog.Warn("visual word index out of range",
				"index", cue.WordIndex,
				"total_words", len(timings),
			)
			continue
		}

		slog.Info("searching image", "query", cue.SearchQuery)
		results, err := p.svc.ImageSearch().Search(ctx, cue.SearchQuery, 1)
		if err != nil {
			slog.Warn("failed to search image",
				"query", cue.SearchQuery,
				"error", err,
			)
			continue
		}

		slog.Info("search results", "query", cue.SearchQuery, "results_count", len(results))

		if len(results) == 0 {
			slog.Warn("no images found", "query", cue.SearchQuery)
			continue
		}

		slog.Info("downloading image",
			"url", results[0].ImageURL,
			"thumbnail", results[0].ThumbURL,
		)

		imageData, err := p.svc.ImageSearch().DownloadImage(ctx, results[0].ImageURL)
		if err != nil {
			slog.Warn("failed to download image",
				"url", results[0].ImageURL,
				"error", err,
			)
			continue
		}

		slog.Info("downloaded image", "size_bytes", len(imageData))

		if !isValidImage(imageData) {
			slog.Warn("invalid image data, skipping", "url", results[0].ImageURL)
			continue
		}

		ext := ".jpg"
		if strings.Contains(results[0].ImageURL, ".png") {
			ext = ".png"
		}
		imagePath := filepath.Join(cfg.Video.OutputDir, fmt.Sprintf("img_%d_%d%s", time.Now().UnixNano(), cue.WordIndex, ext))
		if err := os.WriteFile(imagePath, imageData, 0644); err != nil {
			slog.Warn("failed to save image", "path", imagePath, "error", err)
			continue
		}

		slog.Info("saved image", "path", imagePath)

		startTime := timings[cue.WordIndex].StartTime
		endTime := startTime + cfg.Visuals.DisplayTime

		overlays = append(overlays, video.ImageOverlay{
			ImagePath: imagePath,
			StartTime: startTime,
			EndTime:   endTime,
			Width:     cfg.Visuals.ImageWidth,
			Height:    cfg.Visuals.ImageHeight,
		})

		slog.Info("added image overlay",
			"query", cue.SearchQuery,
			"path", imagePath,
			"start", startTime,
			"end", endTime,
			"width", cfg.Visuals.ImageWidth,
			"height", cfg.Visuals.ImageHeight,
		)
	}

	slog.Info("finished fetching visuals", "total_overlays", len(overlays))
	return overlays
}

func (p *Pipeline) generateConversation(ctx context.Context, topic string) (*GenerateResult, error) {
	cfg := p.svc.Config()

	speakerNames := make([]string, len(cfg.ElevenLabs.Voices))
	voiceMap := make(map[string]elevenlabs.VoiceConfig)
	for i, v := range cfg.ElevenLabs.Voices {
		speakerNames[i] = v.Name
		voiceMap[v.Name] = elevenlabs.VoiceConfig{
			ID:         v.ID,
			Stability:  v.Stability,
			Similarity: v.Similarity,
		}
	}

	slog.Info("generating conversation script", "topic", topic, "speakers", speakerNames)
	script, err := p.svc.DeepSeek().GenerateConversation(ctx, topic, speakerNames, cfg.Content.ScriptLength, cfg.Content.HookDuration)
	if err != nil {
		return nil, fmt.Errorf("failed to generate conversation: %w", err)
	}

	parsed := dialogue.Parse(script)
	if parsed.IsEmpty() {
		slog.Warn("conversation parsing returned no lines, falling back to single voice")
		return p.generateSingleVoice(ctx, topic)
	}

	slog.Info("parsed conversation", "lines", len(parsed.Lines), "speakers", parsed.Speakers())

	segments := make([]video.AudioSegment, 0, len(parsed.Lines))
	for i, line := range parsed.Lines {
		voice, ok := voiceMap[line.Speaker]
		if !ok {
			slog.Warn("unknown speaker, using first voice", "speaker", line.Speaker)
			voice = voiceMap[speakerNames[0]]
		}

		slog.Info("generating speech for line", "line", i+1, "speaker", line.Speaker)
		result, err := p.svc.ElevenLabs().GenerateSpeechWithVoice(ctx, line.Text, voice)
		if err != nil {
			return nil, fmt.Errorf("failed to generate speech for line %d: %w", i+1, err)
		}

		segments = append(segments, video.AudioSegment{
			Audio:   result.Audio,
			Timings: result.Timings,
		})
	}

	stitcher := video.NewAudioStitcher(cfg.Video.OutputDir)
	stitched, err := stitcher.Stitch(ctx, segments)
	if err != nil {
		return nil, fmt.Errorf("failed to stitch audio: %w", err)
	}

	audioFilename := fmt.Sprintf("audio_%d.mp3", time.Now().Unix())
	audioPath, err := p.svc.Storage().SaveAudio(stitched.Data, audioFilename)
	if err != nil {
		return nil, fmt.Errorf("failed to save audio: %w", err)
	}
	slog.Info("saved stitched audio", "path", audioPath, "duration", stitched.Duration)

	var imageOverlays []video.ImageOverlay
	if cfg.Visuals.Enabled && p.svc.ImageSearch() != nil {
		slog.Info("generating visual cues for conversation", "topic", topic)
		visuals := p.generateVisualCues(ctx, parsed.FullText())
		if len(visuals) > 0 {
			imageOverlays = p.fetchVisualImages(ctx, visuals, stitched.Timings)
		}
	}

	assembleReq := video.AssembleRequest{
		AudioPath:     audioPath,
		AudioDuration: stitched.Duration,
		Script:        parsed.FullText(),
		ScriptID:      time.Now().Unix(),
		WordTimings:   stitched.Timings,
		ImageOverlays: imageOverlays,
	}

	slog.Info("assembling video", "duration", stitched.Duration, "overlays", len(imageOverlays))
	result, err := p.svc.Assembler().Assemble(ctx, assembleReq)
	if err != nil {
		return nil, fmt.Errorf("failed to assemble video: %w", err)
	}
	slog.Info("video assembled", "path", result.OutputPath)

	title := p.generateTitle(ctx, script, topic)

	return &GenerateResult{
		Title:         title,
		ScriptContent: script,
		AudioPath:     audioPath,
		VideoPath:     result.OutputPath,
		Duration:      result.Duration,
	}, nil
}

func (p *Pipeline) generateVisualCues(ctx context.Context, script string) []deepseek.VisualCue {
	slog.Info("generating visual cues for existing script", "script_length", len(script))

	visuals, err := p.svc.DeepSeek().GenerateVisualsForScript(ctx, script)
	if err != nil {
		slog.Warn("failed to generate visual cues", "error", err)
		return nil
	}

	slog.Info("generated visual cues",
		"cues_count", len(visuals),
		"cues", visuals,
	)

	return visuals
}

func (p *Pipeline) GenerateFromReddit(ctx context.Context, subreddit string, limit int) (*GenerateResult, error) {
	slog.Info("fetching reddit posts", "subreddit", subreddit, "limit", limit)
	posts, err := p.svc.Reddit().GetTopStories(ctx, subreddit, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch reddit posts: %w", err)
	}

	if len(posts) == 0 {
		return nil, fmt.Errorf("no posts found in subreddit: %s", subreddit)
	}

	post := posts[0]
	slog.Info("selected post", "title", post.Title, "score", post.Score)

	content := post.Title
	if post.Selftext != "" {
		content = post.Title + "\n\n" + post.Selftext
	}

	return p.Generate(ctx, content)
}

func (p *Pipeline) Upload(ctx context.Context, videoPath, title, description string) (*uploader.UploadResponse, error) {
	cfg := p.svc.Config()

	req := uploader.UploadRequest{
		FilePath:    videoPath,
		Title:       title,
		Description: description,
		Tags:        cfg.YouTube.DefaultTags,
		Privacy:     cfg.YouTube.PrivacyStatus,
	}

	slog.Info("uploading video", "path", videoPath, "title", title, "platform", p.svc.Uploader().Platform())
	resp, err := p.svc.Uploader().Upload(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to upload video: %w", err)
	}
	slog.Info("video uploaded", "id", resp.ID, "url", resp.URL)

	return resp, nil
}

func (p *Pipeline) GenerateAndUpload(ctx context.Context, topic string) (*uploader.UploadResponse, error) {
	result, err := p.Generate(ctx, topic)
	if err != nil {
		return nil, err
	}

	title := result.Title + " #shorts"
	description := fmt.Sprintf("A short video about %s\n\n#shorts #facts", topic)

	return p.Upload(ctx, result.VideoPath, title, description)
}

func estimateAudioDuration(script string) float64 {
	words := len(script) / 5
	wordsPerSecond := 2.5
	return float64(words) / wordsPerSecond
}

func getAudioDuration(timings []elevenlabs.WordTiming) float64 {
	if len(timings) == 0 {
		return 0
	}
	return timings[len(timings)-1].EndTime
}

func (p *Pipeline) generateTitle(ctx context.Context, script, fallback string) string {
	slog.Info("generating title from script")
	title, err := p.svc.DeepSeek().GenerateTitle(ctx, script)
	if err != nil {
		slog.Warn("failed to generate title, using fallback", "error", err)
		return fallback
	}
	return title
}

func isValidImage(data []byte) bool {
	if len(data) < 100 {
		return false
	}
	if bytes.HasPrefix(data, []byte{0xFF, 0xD8, 0xFF}) {
		return true
	}
	if bytes.HasPrefix(data, []byte{0x89, 0x50, 0x4E, 0x47}) {
		return true
	}
	_, _, err := image.Decode(bytes.NewReader(data))
	return err == nil
}
