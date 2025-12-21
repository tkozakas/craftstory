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

	"craftstory/internal/dialogue"
	"craftstory/internal/imagesearch"
	"craftstory/internal/llm"
	"craftstory/internal/tts"
	"craftstory/internal/uploader"
	"craftstory/internal/video"
	"craftstory/pkg/config"
)

var (
	sanitizeRegex  = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)
	wordSplitRegex = regexp.MustCompile(`\s+`)
)

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

type UploadRequest struct {
	VideoPath   string
	Title       string
	Description string
}

func NewPipeline(svc *Service) *Pipeline {
	return &Pipeline{svc: svc}
}

func (p *Pipeline) Generate(ctx context.Context, topic string) (*GenerateResult, error) {
	cfg := p.svc.Config()

	if cfg.Content.ConversationMode && len(cfg.ElevenLabs.Voices) >= 2 {
		return p.generateConversation(ctx, topic)
	}
	return p.generateSingleVoice(ctx, topic)
}

func (p *Pipeline) GenerateFromReddit(ctx context.Context, subreddit string, limit int) (*GenerateResult, error) {
	posts, err := p.svc.Reddit().GetTopStories(ctx, subreddit, limit)
	if err != nil {
		return nil, fmt.Errorf("fetch reddit posts: %w", err)
	}

	if len(posts) == 0 {
		return nil, fmt.Errorf("no posts found in subreddit: %s", subreddit)
	}

	post := posts[0]
	content := post.Title
	if post.Selftext != "" {
		content = post.Title + "\n\n" + post.Selftext
	}

	return p.Generate(ctx, content)
}

func (p *Pipeline) Upload(ctx context.Context, req UploadRequest) (*uploader.UploadResponse, error) {
	cfg := p.svc.Config()

	uploadReq := uploader.UploadRequest{
		FilePath:    req.VideoPath,
		Title:       req.Title,
		Description: req.Description,
		Tags:        cfg.YouTube.DefaultTags,
		Privacy:     cfg.YouTube.PrivacyStatus,
	}

	resp, err := p.svc.Uploader().Upload(ctx, uploadReq)
	if err != nil {
		return nil, fmt.Errorf("upload video: %w", err)
	}
	return resp, nil
}

func (p *Pipeline) GenerateAndUpload(ctx context.Context, topic string) (*uploader.UploadResponse, error) {
	result, err := p.Generate(ctx, topic)
	if err != nil {
		return nil, err
	}

	return p.Upload(ctx, UploadRequest{
		VideoPath:   result.VideoPath,
		Title:       result.Title + " #shorts",
		Description: fmt.Sprintf("A short video about %s\n\n#shorts #facts", topic),
	})
}

func (p *Pipeline) GenerateBatch(ctx context.Context, topics []string) *BatchResult {
	result := &BatchResult{
		Results: make([]*GenerateResult, 0, len(topics)),
		Errors:  make([]BatchError, 0),
	}

	for _, topic := range topics {
		genResult, err := p.Generate(ctx, topic)
		if err != nil {
			result.Errors = append(result.Errors, BatchError{Topic: topic, Error: err})
			result.Failed++
			continue
		}
		result.Results = append(result.Results, genResult)
		result.Succeeded++
	}

	return result
}

func (p *Pipeline) GenerateBatchFromReddit(ctx context.Context, subreddit string, limit int) *BatchResult {
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
		resp, err := p.Upload(ctx, UploadRequest{
			VideoPath:   result.VideoPath,
			Title:       result.Title + " #shorts",
			Description: "Generated video\n\n#shorts #facts",
		})
		if err != nil {
			allErrors = append(allErrors, BatchError{Topic: result.Title, Error: err})
			continue
		}
		responses = append(responses, resp)
	}

	return responses, allErrors
}

func (p *Pipeline) generateSingleVoice(ctx context.Context, topic string) (*GenerateResult, error) {
	cfg := p.svc.Config()
	sess := newSession(cfg.Video.OutputDir)

	slog.Info("Generating script...")
	script, visuals, err := p.generateScript(ctx, topic)
	if err != nil {
		return nil, err
	}

	title := p.generateTitle(ctx, script, topic)
	if err := sess.finalize(title); err != nil {
		return nil, err
	}

	_ = os.WriteFile(sess.scriptPath(), []byte(script), 0644)

	slog.Info("Generating speech...", "length", len(script))
	speechResult, err := p.svc.TTS().GenerateSpeechWithTimings(ctx, script)
	if err != nil {
		return nil, fmt.Errorf("generate speech: %w", err)
	}

	if err := os.WriteFile(sess.audioPath(), speechResult.Audio, 0644); err != nil {
		return nil, fmt.Errorf("save audio: %w", err)
	}

	slog.Info("Fetching images...")
	overlays := p.fetchVisualImages(ctx, sess, script, visuals, speechResult.Timings)

	slog.Info("Assembling video...")
	result, err := p.assembleVideo(ctx, sess, script, speechResult, overlays, nil)
	if err != nil {
		return nil, err
	}

	return &GenerateResult{
		Title:         title,
		ScriptContent: script,
		OutputDir:     sess.dir,
		AudioPath:     sess.audioPath(),
		VideoPath:     result.OutputPath,
		Duration:      result.Duration,
	}, nil
}

func (p *Pipeline) generateConversation(ctx context.Context, topic string) (*GenerateResult, error) {
	cfg := p.svc.Config()
	sess := newSession(cfg.Video.OutputDir)

	voices := p.getConversationVoices()
	voiceMap := buildVoiceMap(voices)

	slog.Info("Generating conversation script...")
	script, err := p.generateConversationScript(ctx, topic, voices)
	if err != nil {
		return nil, err
	}

	parsed := dialogue.Parse(script)
	if parsed.IsEmpty() {
		return p.generateSingleVoice(ctx, topic)
	}

	title := p.generateTitle(ctx, script, topic)
	if err := sess.finalize(title); err != nil {
		return nil, err
	}

	_ = os.WriteFile(sess.scriptPath(), []byte(script), 0644)

	slog.Info("Generating speech segments...", "lines", len(parsed.Lines))
	segments, err := p.generateSpeechSegments(ctx, parsed, voiceMap, voices[0].Name)
	if err != nil {
		return nil, err
	}

	slog.Info("Stitching audio...")
	stitched, err := video.NewAudioStitcher(sess.dir).Stitch(ctx, segments)
	if err != nil {
		return nil, fmt.Errorf("stitch audio: %w", err)
	}

	if err := os.WriteFile(sess.audioPath(), stitched.Data, 0644); err != nil {
		return nil, fmt.Errorf("save audio: %w", err)
	}

	charOverlays := buildCharacterOverlays(stitched.Segments, voiceMap)

	slog.Info("Fetching images...")
	imageOverlays := p.fetchConversationVisuals(ctx, sess, parsed, stitched)

	slog.Info("Assembling video...")
	result, err := p.assembleConversation(ctx, sess, parsed, stitched, imageOverlays, charOverlays)
	if err != nil {
		return nil, err
	}

	return &GenerateResult{
		Title:         title,
		ScriptContent: script,
		OutputDir:     sess.dir,
		AudioPath:     sess.audioPath(),
		VideoPath:     result.OutputPath,
		Duration:      result.Duration,
	}, nil
}

func (p *Pipeline) generateScript(ctx context.Context, topic string) (string, []llm.VisualCue, error) {
	cfg := p.svc.Config()

	if !cfg.Visuals.Enabled || p.svc.ImageSearch() == nil {
		script, err := p.svc.LLM().GenerateScript(ctx, topic, cfg.Content.ScriptLength, cfg.Content.HookDuration)
		return script, nil, err
	}

	result, err := p.svc.LLM().GenerateScriptWithVisuals(ctx, topic, cfg.Content.ScriptLength, cfg.Content.HookDuration)
	if err != nil {
		return "", nil, fmt.Errorf("generate script: %w", err)
	}
	return result.Script, result.Visuals, nil
}

func (p *Pipeline) generateConversationScript(ctx context.Context, topic string, voices []tts.VoiceConfig) (string, error) {
	cfg := p.svc.Config()

	speakerNames := make([]string, len(voices))
	for i, v := range voices {
		speakerNames[i] = v.Name
	}

	script, err := p.svc.LLM().GenerateConversation(ctx, topic, speakerNames, cfg.Content.ScriptLength, cfg.Content.HookDuration)
	if err != nil {
		return "", fmt.Errorf("generate conversation: %w", err)
	}
	return script, nil
}

func (p *Pipeline) generateTitle(ctx context.Context, script, fallback string) string {
	title, err := p.svc.LLM().GenerateTitle(ctx, script)
	if err != nil {
		return fallback
	}
	return title
}

func (p *Pipeline) getConversationVoices() []tts.VoiceConfig {
	cfg := p.svc.Config()
	voices := cfg.ElevenLabs.Voices

	result := make([]tts.VoiceConfig, len(voices))
	for i, v := range voices {
		result[i] = tts.VoiceConfig{
			ID:         v.ID,
			Name:       v.Name,
			Avatar:     v.Avatar,
			Stability:  v.Stability,
			Similarity: v.Similarity,
		}
	}
	return result
}

func (p *Pipeline) generateSpeechSegments(ctx context.Context, parsed *dialogue.Script, voiceMap map[string]tts.VoiceConfig, defaultSpeaker string) ([]video.AudioSegment, error) {
	segments := make([]video.AudioSegment, len(parsed.Lines))
	total := len(parsed.Lines)

	// Process sequentially for consistent audio quality
	for i, line := range parsed.Lines {
		voice, ok := voiceMap[line.Speaker]
		if !ok {
			slog.Warn("unknown speaker, using default", "speaker", line.Speaker)
			voice = voiceMap[defaultSpeaker]
		}

		slog.Info("Generating speech", "line", i+1, "total", total, "speaker", line.Speaker)

		result, err := p.svc.TTS().GenerateSpeechWithVoice(ctx, line.Text, voice)
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

func (p *Pipeline) fetchVisualImages(ctx context.Context, sess *session, script string, visuals []llm.VisualCue, timings []tts.WordTiming) []video.ImageOverlay {
	if p.svc.ImageSearch() == nil {
		slog.Debug("Image search client not configured")
		return nil
	}
	if len(visuals) == 0 {
		slog.Debug("No visual cues provided")
		return nil
	}

	slog.Info("Processing visual cues", "count", len(visuals))
	cfg := p.svc.Config()
	overlays := make([]video.ImageOverlay, 0, len(visuals))

	for i, cue := range visuals {
		slog.Debug("Processing cue", "index", i, "keyword", cue.Keyword, "query", cue.SearchQuery)
		overlay := p.fetchSingleImage(ctx, sess, i, cue, script, timings, cfg)
		if overlay != nil {
			overlays = append(overlays, *overlay)
			slog.Info("Fetched image", "keyword", cue.Keyword, "path", overlay.ImagePath)
		} else {
			slog.Warn("Failed to fetch image", "keyword", cue.Keyword, "query", cue.SearchQuery)
		}
	}

	slog.Info("Image fetch complete", "total", len(visuals), "success", len(overlays))
	return p.enforceImageConstraints(overlays)
}

func (p *Pipeline) fetchSingleImage(ctx context.Context, sess *session, index int, cue llm.VisualCue, script string, timings []tts.WordTiming, cfg *config.Config) *video.ImageOverlay {
	wordIndex := findKeywordIndex(script, cue.Keyword)
	if wordIndex < 0 || wordIndex >= len(timings) {
		slog.Debug("Keyword not found in timings", "keyword", cue.Keyword, "wordIndex", wordIndex, "timingsLen", len(timings))
		return nil
	}

	results, err := p.svc.ImageSearch().Search(ctx, cue.SearchQuery, 5)
	if err != nil {
		slog.Debug("Image search failed", "query", cue.SearchQuery, "error", err)
		return nil
	}
	if len(results) == 0 {
		slog.Debug("No search results", "query", cue.SearchQuery)
		return nil
	}

	imageData, ext := p.downloadValidImage(ctx, results)
	if imageData == nil {
		return nil
	}

	imagePath := sess.imagePath(index, ext)
	if err := os.WriteFile(imagePath, imageData, 0644); err != nil {
		return nil
	}

	return &video.ImageOverlay{
		ImagePath: imagePath,
		StartTime: timings[wordIndex].StartTime,
		EndTime:   timings[wordIndex].StartTime + cfg.Visuals.DisplayTime,
		Width:     cfg.Visuals.ImageWidth,
		Height:    cfg.Visuals.ImageHeight,
	}
}

func (p *Pipeline) downloadValidImage(ctx context.Context, results []imagesearch.SearchResult) ([]byte, string) {
	for i, result := range results {
		slog.Debug("Trying to download image", "index", i, "url", result.ImageURL)
		data, err := p.svc.ImageSearch().DownloadImage(ctx, result.ImageURL)
		if err != nil {
			slog.Debug("Download failed", "error", err)
			continue
		}

		if !isValidImage(data) {
			slog.Debug("Invalid image format", "size", len(data))
			continue
		}
		if len(data) < 10000 {
			slog.Debug("Image too small", "size", len(data))
			continue
		}

		ext := ".jpg"
		if strings.Contains(result.ImageURL, ".png") {
			ext = ".png"
		}
		slog.Debug("Image downloaded successfully", "size", len(data))
		return data, ext
	}
	return nil, ""
}

func (p *Pipeline) fetchConversationVisuals(ctx context.Context, sess *session, parsed *dialogue.Script, stitched *video.StitchedAudio) []video.ImageOverlay {
	cfg := p.svc.Config()
	if !cfg.Visuals.Enabled || p.svc.ImageSearch() == nil {
		return nil
	}

	fullText := parsed.FullText()
	visuals := p.generateVisualCues(ctx, fullText)
	if len(visuals) == 0 {
		return nil
	}

	return p.fetchVisualImages(ctx, sess, fullText, visuals, stitched.Timings)
}

func (p *Pipeline) generateVisualCues(ctx context.Context, script string) []llm.VisualCue {
	visuals, err := p.svc.LLM().GenerateVisualsForScript(ctx, script)
	if err != nil {
		return nil
	}
	return visuals
}

func (p *Pipeline) assembleVideo(ctx context.Context, sess *session, script string, speech *tts.SpeechResult, imageOverlays []video.ImageOverlay, charOverlays []video.CharacterOverlay) (*video.AssembleResult, error) {
	cfg := p.svc.Config()
	duration := audioDuration(speech.Timings)
	maxDuration := cfg.Video.MaxDuration
	if maxDuration > 0 && duration > maxDuration {
		return nil, fmt.Errorf("audio duration %.1fs exceeds limit of %.0fs", duration, maxDuration)
	}

	req := video.AssembleRequest{
		AudioPath:         sess.audioPath(),
		AudioDuration:     duration,
		Script:            script,
		OutputPath:        sess.videoPath(),
		WordTimings:       speech.Timings,
		ImageOverlays:     imageOverlays,
		CharacterOverlays: charOverlays,
	}

	result, err := p.svc.Assembler().Assemble(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("assemble video: %w", err)
	}
	return result, nil
}

func (p *Pipeline) assembleConversation(ctx context.Context, sess *session, parsed *dialogue.Script, stitched *video.StitchedAudio, imageOverlays []video.ImageOverlay, charOverlays []video.CharacterOverlay) (*video.AssembleResult, error) {
	req := video.AssembleRequest{
		AudioPath:         sess.audioPath(),
		AudioDuration:     stitched.Duration,
		Script:            parsed.FullText(),
		OutputPath:        sess.videoPath(),
		WordTimings:       stitched.Timings,
		ImageOverlays:     imageOverlays,
		CharacterOverlays: charOverlays,
	}

	result, err := p.svc.Assembler().Assemble(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("assemble video: %w", err)
	}
	return result, nil
}

func (p *Pipeline) enforceImageConstraints(overlays []video.ImageOverlay) []video.ImageOverlay {
	if len(overlays) <= 1 {
		return overlays
	}

	minGap := p.svc.Config().Visuals.MinGap
	filtered := make([]video.ImageOverlay, 0, len(overlays))
	filtered = append(filtered, overlays[0])

	for i := 1; i < len(overlays); i++ {
		gap := overlays[i].StartTime - filtered[len(filtered)-1].EndTime
		if gap >= minGap {
			filtered = append(filtered, overlays[i])
		}
	}
	return filtered
}

func buildVoiceMap(voices []tts.VoiceConfig) map[string]tts.VoiceConfig {
	m := make(map[string]tts.VoiceConfig, len(voices))
	for _, v := range voices {
		m[v.Name] = v
	}
	return m
}

func buildCharacterOverlays(segments []video.SegmentInfo, voiceMap map[string]tts.VoiceConfig) []video.CharacterOverlay {
	speakerPositions := make(map[string]int)
	nextPosition := 0

	var overlays []video.CharacterOverlay
	for _, seg := range segments {
		voice, ok := voiceMap[seg.Speaker]
		if !ok || voice.Avatar == "" {
			continue
		}

		pos, exists := speakerPositions[seg.Speaker]
		if !exists {
			pos = nextPosition
			speakerPositions[seg.Speaker] = pos
			nextPosition = (nextPosition + 1) % 2
		}

		overlays = append(overlays, video.CharacterOverlay{
			Speaker:    seg.Speaker,
			AvatarPath: voice.Avatar,
			StartTime:  seg.StartTime,
			EndTime:    seg.EndTime,
			Position:   pos,
		})
	}
	return overlays
}

type session struct {
	id      string
	dir     string
	baseDir string
}

func newSession(baseDir string) *session {
	return &session{
		id:      time.Now().Format("20060102_150405"),
		baseDir: baseDir,
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
	return os.MkdirAll(s.dir, 0755)
}

func (s *session) audioPath() string  { return filepath.Join(s.dir, "audio.mp3") }
func (s *session) videoPath() string  { return filepath.Join(s.dir, "video.mp4") }
func (s *session) scriptPath() string { return filepath.Join(s.dir, "script.txt") }

func (s *session) imagePath(index int, ext string) string {
	return filepath.Join(s.dir, fmt.Sprintf("image_%d%s", index, ext))
}

func sanitizeForPath(s string) string {
	s = strings.ToLower(s)
	s = sanitizeRegex.ReplaceAllString(s, "_")
	return strings.Trim(s, "_")
}

func findKeywordIndex(script, keyword string) int {
	if keyword == "" {
		return -1
	}

	words := wordSplitRegex.Split(script, -1)
	keywordLower := strings.ToLower(keyword)
	keywordWords := wordSplitRegex.Split(keywordLower, -1)

	if len(keywordWords) == 1 {
		return findSingleKeyword(words, keywordLower)
	}
	return findMultiWordKeyword(words, keywordWords)
}

func findSingleKeyword(words []string, keyword string) int {
	for i, word := range words {
		if cleanWord(word) == keyword {
			return i
		}
	}
	for i, word := range words {
		if strings.Contains(cleanWord(word), keyword) {
			return i
		}
	}
	return -1
}

func findMultiWordKeyword(words, keywordWords []string) int {
	for i := 0; i <= len(words)-len(keywordWords); i++ {
		if matchesAt(words, keywordWords, i) {
			return i
		}
	}
	return -1
}

func matchesAt(words, keywordWords []string, start int) bool {
	for j, kw := range keywordWords {
		if cleanWord(words[start+j]) != kw {
			return false
		}
	}
	return true
}

func cleanWord(word string) string {
	return strings.ToLower(strings.Trim(word, ".,!?;:'\"()[]{}"))
}

func audioDuration(timings []tts.WordTiming) float64 {
	if len(timings) == 0 {
		return 0
	}
	return timings[len(timings)-1].EndTime
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
