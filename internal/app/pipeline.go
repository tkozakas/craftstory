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
	"regexp"
	"strings"
	"time"

	"craftstory/internal/deepseek"
	"craftstory/internal/dialogue"
	"craftstory/internal/elevenlabs"
	"craftstory/internal/uploader"
	"craftstory/internal/video"
)

var sanitizeRegex = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

type Pipeline struct {
	svc *Service
}

type GenerateResult struct {
	Title         string
	ScriptContent string
	OutputDir     string
	AudioPath     string
	VideoPath     string
	Duration      float64
}

type BatchResult struct {
	Results   []*GenerateResult
	Errors    []BatchError
	Succeeded int
	Failed    int
}

type BatchError struct {
	Topic string
	Error error
}

type session struct {
	id        string
	dir       string
	baseDir   string
	timestamp time.Time
}

func newSession(baseDir string) *session {
	ts := time.Now()
	id := ts.Format("20060102_150405")
	return &session{
		id:        id,
		baseDir:   baseDir,
		timestamp: ts,
	}
}

func (s *session) finalize(title string) error {
	sanitized := sanitizeForPath(title)
	if sanitized == "" {
		sanitized = "untitled"
	}
	if len(sanitized) > 50 {
		sanitized = sanitized[:50]
	}

	s.dir = filepath.Join(s.baseDir, fmt.Sprintf("%s_%s", s.id, sanitized))
	if err := os.MkdirAll(s.dir, 0755); err != nil {
		return fmt.Errorf("failed to create session directory: %w", err)
	}
	slog.Info("created session directory", "path", s.dir)
	return nil
}

func (s *session) audioPath() string {
	return filepath.Join(s.dir, "audio.mp3")
}

func (s *session) videoPath() string {
	return filepath.Join(s.dir, "video.mp4")
}

func (s *session) scriptPath() string {
	return filepath.Join(s.dir, "script.txt")
}

func (s *session) imagePath(index int, ext string) string {
	return filepath.Join(s.dir, fmt.Sprintf("image_%d%s", index, ext))
}

func sanitizeForPath(s string) string {
	s = strings.ToLower(s)
	s = sanitizeRegex.ReplaceAllString(s, "_")
	s = strings.Trim(s, "_")
	return s
}

// wordSplitRegex matches word boundaries for splitting script into words.
var wordSplitRegex = regexp.MustCompile(`\s+`)

// findKeywordIndex searches for a keyword in the script and returns the 0-based word index.
// It performs case-insensitive matching and handles punctuation attached to words.
// Returns -1 if the keyword is not found.
func findKeywordIndex(script, keyword string) int {
	if keyword == "" {
		return -1
	}

	words := wordSplitRegex.Split(script, -1)
	keywordLower := strings.ToLower(keyword)
	keywordWords := wordSplitRegex.Split(keywordLower, -1)

	// Single word keyword
	if len(keywordWords) == 1 {
		for i, word := range words {
			// Strip common punctuation from word for comparison
			cleanWord := strings.ToLower(strings.Trim(word, ".,!?;:'\"()[]{}"))
			if cleanWord == keywordLower {
				return i
			}
		}
		// Try partial match (keyword contained in word)
		for i, word := range words {
			cleanWord := strings.ToLower(strings.Trim(word, ".,!?;:'\"()[]{}"))
			if strings.Contains(cleanWord, keywordLower) {
				return i
			}
		}
	} else {
		// Multi-word keyword - find first word then verify sequence
		for i := 0; i <= len(words)-len(keywordWords); i++ {
			match := true
			for j, kw := range keywordWords {
				cleanWord := strings.ToLower(strings.Trim(words[i+j], ".,!?;:'\"()[]{}"))
				if cleanWord != kw {
					match = false
					break
				}
			}
			if match {
				return i
			}
		}
	}

	return -1
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
	sess := newSession(cfg.Video.OutputDir)

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

	title := p.generateTitle(ctx, script, topic)

	if err := sess.finalize(title); err != nil {
		return nil, err
	}

	if err := os.WriteFile(sess.scriptPath(), []byte(script), 0644); err != nil {
		slog.Warn("failed to save script file", "error", err)
	}

	slog.Info("generating speech", "script_length", len(script))
	speechResult, err := p.svc.ElevenLabs().GenerateSpeechWithTimings(ctx, script)
	if err != nil {
		return nil, fmt.Errorf("failed to generate speech: %w", err)
	}

	audioPath := sess.audioPath()
	if err := os.WriteFile(audioPath, speechResult.Audio, 0644); err != nil {
		return nil, fmt.Errorf("failed to save audio: %w", err)
	}
	slog.Info("saved audio", "path", audioPath)

	audioDuration := getAudioDuration(speechResult.Timings)

	var imageOverlays []video.ImageOverlay
	if len(visuals) > 0 {
		imageOverlays = p.fetchVisualImagesWithSession(ctx, sess, script, visuals, speechResult.Timings)
	}

	assembleReq := video.AssembleRequest{
		AudioPath:     audioPath,
		AudioDuration: audioDuration,
		Script:        script,
		OutputPath:    sess.videoPath(),
		WordTimings:   speechResult.Timings,
		ImageOverlays: imageOverlays,
	}

	slog.Info("assembling video", "duration", audioDuration, "overlays", len(imageOverlays))
	result, err := p.svc.Assembler().Assemble(ctx, assembleReq)
	if err != nil {
		return nil, fmt.Errorf("failed to assemble video: %w", err)
	}
	slog.Info("video assembled", "path", result.OutputPath)

	return &GenerateResult{
		Title:         title,
		ScriptContent: script,
		OutputDir:     sess.dir,
		AudioPath:     audioPath,
		VideoPath:     result.OutputPath,
		Duration:      result.Duration,
	}, nil
}

func (p *Pipeline) fetchVisualImagesWithSession(ctx context.Context, sess *session, script string, visuals []deepseek.VisualCue, timings []elevenlabs.WordTiming) []video.ImageOverlay {
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
		// Find word index from keyword
		wordIndex := findKeywordIndex(script, cue.Keyword)
		if wordIndex < 0 {
			slog.Warn("keyword not found in script",
				"keyword", cue.Keyword,
				"query", cue.SearchQuery,
			)
			continue
		}

		slog.Info("processing visual cue",
			"index", i,
			"keyword", cue.Keyword,
			"query", cue.SearchQuery,
			"word_index", wordIndex,
		)

		if wordIndex >= len(timings) {
			slog.Warn("visual word index out of range",
				"index", wordIndex,
				"total_words", len(timings),
			)
			continue
		}

		slog.Info("searching image", "query", cue.SearchQuery)
		results, err := p.svc.ImageSearch().Search(ctx, cue.SearchQuery, 5)
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

		var imageData []byte
		var selectedResult int
		for j, result := range results {
			slog.Info("trying image",
				"index", j,
				"url", result.ImageURL,
				"width", result.Width,
				"height", result.Height,
			)

			data, err := p.svc.ImageSearch().DownloadImage(ctx, result.ImageURL)
			if err != nil {
				slog.Warn("failed to download image",
					"url", result.ImageURL,
					"error", err,
				)
				continue
			}

			if !isValidImage(data) {
				slog.Warn("invalid image data, trying next", "url", result.ImageURL)
				continue
			}

			if len(data) < 10000 {
				slog.Warn("image too small, trying next", "url", result.ImageURL, "size", len(data))
				continue
			}

			imageData = data
			selectedResult = j
			slog.Info("downloaded quality image",
				"url", result.ImageURL,
				"size_bytes", len(data),
				"width", result.Width,
				"height", result.Height,
			)
			break
		}

		if imageData == nil {
			slog.Warn("could not download any valid image for query", "query", cue.SearchQuery)
			continue
		}

		ext := ".jpg"
		if strings.Contains(results[selectedResult].ImageURL, ".png") {
			ext = ".png"
		}
		imagePath := sess.imagePath(i, ext)
		if err := os.WriteFile(imagePath, imageData, 0644); err != nil {
			slog.Warn("failed to save image", "path", imagePath, "error", err)
			continue
		}

		slog.Info("saved image", "path", imagePath)

		startTime := timings[wordIndex].StartTime
		displayDuration := cfg.Visuals.DisplayTime
		endTime := startTime + displayDuration

		overlays = append(overlays, video.ImageOverlay{
			ImagePath: imagePath,
			StartTime: startTime,
			EndTime:   endTime,
			Width:     cfg.Visuals.ImageWidth,
			Height:    cfg.Visuals.ImageHeight,
		})

		slog.Info("added image overlay",
			"keyword", cue.Keyword,
			"query", cue.SearchQuery,
			"path", imagePath,
			"start", startTime,
			"end", endTime,
			"duration", displayDuration,
			"width", cfg.Visuals.ImageWidth,
			"height", cfg.Visuals.ImageHeight,
		)
	}

	slog.Info("finished fetching visuals", "total_overlays", len(overlays))
	overlays = p.enforceImageConstraints(overlays)

	return overlays
}

func (p *Pipeline) generateConversation(ctx context.Context, topic string) (*GenerateResult, error) {
	cfg := p.svc.Config()
	sess := newSession(cfg.Video.OutputDir)

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

	title := p.generateTitle(ctx, script, topic)

	if err := sess.finalize(title); err != nil {
		return nil, err
	}

	if err := os.WriteFile(sess.scriptPath(), []byte(script), 0644); err != nil {
		slog.Warn("failed to save script file", "error", err)
	}

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

	stitcher := video.NewAudioStitcher(sess.dir)
	stitched, err := stitcher.Stitch(ctx, segments)
	if err != nil {
		return nil, fmt.Errorf("failed to stitch audio: %w", err)
	}

	audioPath := sess.audioPath()
	if err := os.WriteFile(audioPath, stitched.Data, 0644); err != nil {
		return nil, fmt.Errorf("failed to save audio: %w", err)
	}
	slog.Info("saved stitched audio", "path", audioPath, "duration", stitched.Duration)

	var imageOverlays []video.ImageOverlay
	if cfg.Visuals.Enabled && p.svc.ImageSearch() != nil {
		slog.Info("generating visual cues for conversation", "topic", topic)
		fullText := parsed.FullText()
		visuals := p.generateVisualCues(ctx, fullText)
		if len(visuals) > 0 {
			imageOverlays = p.fetchVisualImagesWithSession(ctx, sess, fullText, visuals, stitched.Timings)
		}
	}

	assembleReq := video.AssembleRequest{
		AudioPath:     audioPath,
		AudioDuration: stitched.Duration,
		Script:        parsed.FullText(),
		OutputPath:    sess.videoPath(),
		WordTimings:   stitched.Timings,
		ImageOverlays: imageOverlays,
	}

	slog.Info("assembling video", "duration", stitched.Duration, "overlays", len(imageOverlays))
	result, err := p.svc.Assembler().Assemble(ctx, assembleReq)
	if err != nil {
		return nil, fmt.Errorf("failed to assemble video: %w", err)
	}
	slog.Info("video assembled", "path", result.OutputPath)

	return &GenerateResult{
		Title:         title,
		ScriptContent: script,
		OutputDir:     sess.dir,
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

func (p *Pipeline) GenerateBatch(ctx context.Context, topics []string) *BatchResult {
	result := &BatchResult{
		Results: make([]*GenerateResult, 0, len(topics)),
		Errors:  make([]BatchError, 0),
	}

	for i, topic := range topics {
		slog.Info("batch generation progress",
			"current", i+1,
			"total", len(topics),
			"topic", topic,
		)

		genResult, err := p.Generate(ctx, topic)
		if err != nil {
			slog.Error("batch generation failed for topic",
				"topic", topic,
				"error", err,
			)
			result.Errors = append(result.Errors, BatchError{
				Topic: topic,
				Error: err,
			})
			result.Failed++
			continue
		}

		result.Results = append(result.Results, genResult)
		result.Succeeded++

		slog.Info("batch generation succeeded",
			"topic", topic,
			"video", genResult.VideoPath,
			"duration", genResult.Duration,
		)
	}

	slog.Info("batch generation complete",
		"succeeded", result.Succeeded,
		"failed", result.Failed,
		"total", len(topics),
	)

	return result
}

func (p *Pipeline) GenerateBatchFromReddit(ctx context.Context, subreddit string, limit int) *BatchResult {
	slog.Info("fetching reddit posts for batch", "subreddit", subreddit, "limit", limit)
	posts, err := p.svc.Reddit().GetTopStories(ctx, subreddit, limit)
	if err != nil {
		return &BatchResult{
			Errors: []BatchError{{Topic: subreddit, Error: err}},
			Failed: 1,
		}
	}

	if len(posts) == 0 {
		return &BatchResult{
			Errors: []BatchError{{Topic: subreddit, Error: fmt.Errorf("no posts found")}},
			Failed: 1,
		}
	}

	topics := make([]string, 0, len(posts))
	for _, post := range posts {
		content := post.Title
		if post.Selftext != "" {
			content = post.Title + "\n\n" + post.Selftext
		}
		topics = append(topics, content)
	}

	return p.GenerateBatch(ctx, topics)
}

func (p *Pipeline) GenerateBatchAndUpload(ctx context.Context, topics []string) ([]*uploader.UploadResponse, []BatchError) {
	batchResult := p.GenerateBatch(ctx, topics)

	responses := make([]*uploader.UploadResponse, 0, len(batchResult.Results))
	allErrors := append([]BatchError{}, batchResult.Errors...)

	for _, result := range batchResult.Results {
		title := result.Title + " #shorts"
		description := "Generated video\n\n#shorts #facts"

		resp, err := p.Upload(ctx, result.VideoPath, title, description)
		if err != nil {
			slog.Error("failed to upload video",
				"video", result.VideoPath,
				"error", err,
			)
			allErrors = append(allErrors, BatchError{
				Topic: result.Title,
				Error: err,
			})
			continue
		}

		slog.Info("video uploaded",
			"title", result.Title,
			"url", resp.URL,
		)
		responses = append(responses, resp)
	}

	return responses, allErrors
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

// enforceImageConstraints filters overlays to ensure minimum gap between images.
// It keeps images that are at least MinGap seconds apart (end of previous to start of next).
func (p *Pipeline) enforceImageConstraints(overlays []video.ImageOverlay) []video.ImageOverlay {
	if len(overlays) <= 1 {
		return overlays
	}

	cfg := p.svc.Config()
	minGap := cfg.Visuals.MinGap

	filtered := make([]video.ImageOverlay, 0, len(overlays))
	filtered = append(filtered, overlays[0])

	for i := 1; i < len(overlays); i++ {
		lastEnd := filtered[len(filtered)-1].EndTime
		currentStart := overlays[i].StartTime

		gap := currentStart - lastEnd
		if gap >= minGap {
			filtered = append(filtered, overlays[i])
			slog.Info("kept image overlay",
				"index", i,
				"start", overlays[i].StartTime,
				"gap_from_previous", gap,
			)
		} else {
			slog.Info("skipped image overlay (too close)",
				"index", i,
				"start", overlays[i].StartTime,
				"gap_from_previous", gap,
				"min_gap_required", minGap,
			)
		}
	}

	slog.Info("enforced image constraints",
		"original_count", len(overlays),
		"filtered_count", len(filtered),
		"min_gap", minGap,
	)

	return filtered
}
