package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"craftstory/internal/uploader"
	"craftstory/internal/video"
)

type Pipeline struct {
	svc *Service
}

type GenerateResult struct {
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

	slog.Info("generating script", "topic", topic)
	script, err := p.svc.DeepSeek().GenerateScript(ctx, topic, cfg.Content.ScriptLength, cfg.Content.HookDuration)
	if err != nil {
		return nil, fmt.Errorf("failed to generate script: %w", err)
	}

	slog.Info("generating speech", "script_length", len(script))
	audioData, err := p.svc.ElevenLabs().GenerateSpeech(ctx, script)
	if err != nil {
		return nil, fmt.Errorf("failed to generate speech: %w", err)
	}

	audioFilename := fmt.Sprintf("audio_%d.mp3", time.Now().Unix())
	audioPath, err := p.svc.Storage().SaveAudio(audioData, audioFilename)
	if err != nil {
		return nil, fmt.Errorf("failed to save audio: %w", err)
	}
	slog.Info("saved audio", "path", audioPath)

	audioDuration := estimateAudioDuration(script)

	assembleReq := video.AssembleRequest{
		AudioPath:     audioPath,
		AudioDuration: audioDuration,
		Script:        script,
		ScriptID:      time.Now().Unix(),
	}

	slog.Info("assembling video", "duration", audioDuration)
	result, err := p.svc.Assembler().Assemble(ctx, assembleReq)
	if err != nil {
		return nil, fmt.Errorf("failed to assemble video: %w", err)
	}
	slog.Info("video assembled", "path", result.OutputPath)

	return &GenerateResult{
		ScriptContent: script,
		AudioPath:     audioPath,
		VideoPath:     result.OutputPath,
		Duration:      result.Duration,
	}, nil
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

	slog.Info("generating title from script")
	title, err := p.svc.DeepSeek().GenerateTitle(ctx, result.ScriptContent)
	if err != nil {
		slog.Warn("failed to generate title, using topic", "error", err)
		title = topic
	}

	title = title + " #shorts"
	description := fmt.Sprintf("A short video about %s\n\n#shorts #facts", topic)

	return p.Upload(ctx, result.VideoPath, title, description)
}

func estimateAudioDuration(script string) float64 {
	words := len(script) / 5
	wordsPerSecond := 2.5
	return float64(words) / wordsPerSecond
}
