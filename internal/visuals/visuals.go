package visuals

import (
	"context"
	"log/slog"
	"os"
	"strings"

	"craftstory/internal/imagesearch"
	"craftstory/internal/llm"
	"craftstory/internal/tts"
	"craftstory/internal/video"
)

type ImageSearcher interface {
	Search(ctx context.Context, query string, count int) ([]imagesearch.SearchResult, error)
	DownloadImage(ctx context.Context, imageURL string) ([]byte, error)
}

type LLMClient interface {
	GenerateVisualsForScript(ctx context.Context, script string) ([]llm.VisualCue, error)
}

type Config struct {
	Enabled     bool
	DisplayTime float64
	ImageWidth  int
	ImageHeight int
	MinGap      float64
}

type FetchRequest struct {
	Script   string
	Visuals  []llm.VisualCue
	Timings  []tts.WordTiming
	ImageDir string
}

type Fetcher struct {
	imageSearch ImageSearcher
	llm         LLMClient
	cfg         Config
}

func NewFetcher(imageSearch ImageSearcher, llm LLMClient, cfg Config) *Fetcher {
	return &Fetcher{
		imageSearch: imageSearch,
		llm:         llm,
		cfg:         cfg,
	}
}

func (f *Fetcher) Fetch(ctx context.Context, req FetchRequest) []video.ImageOverlay {
	if f.imageSearch == nil {
		slog.Debug("Image search client not configured")
		return nil
	}
	if len(req.Visuals) == 0 {
		slog.Debug("No visual cues provided")
		return nil
	}

	slog.Info("Processing visual cues", "count", len(req.Visuals))
	overlays := make([]video.ImageOverlay, 0, len(req.Visuals))

	for i, cue := range req.Visuals {
		slog.Debug("Processing cue", "index", i, "keyword", cue.Keyword, "query", cue.SearchQuery)
		overlay := f.fetchSingle(ctx, req.ImageDir, i, cue, req.Script, req.Timings)
		if overlay != nil {
			overlays = append(overlays, *overlay)
			slog.Info("Fetched image", "keyword", cue.Keyword, "path", overlay.ImagePath)
		} else {
			slog.Warn("Failed to fetch image", "keyword", cue.Keyword, "query", cue.SearchQuery)
		}
	}

	slog.Info("Image fetch complete", "total", len(req.Visuals), "success", len(overlays))
	return f.enforceConstraints(overlays)
}

func (f *Fetcher) FetchForConversation(ctx context.Context, script string, timings []tts.WordTiming, imageDir string) []video.ImageOverlay {
	if !f.cfg.Enabled || f.imageSearch == nil {
		return nil
	}

	visuals := f.generateCues(ctx, script)
	if len(visuals) == 0 {
		return nil
	}

	return f.Fetch(ctx, FetchRequest{
		Script:   script,
		Visuals:  visuals,
		Timings:  timings,
		ImageDir: imageDir,
	})
}

func (f *Fetcher) fetchSingle(ctx context.Context, imageDir string, index int, cue llm.VisualCue, script string, timings []tts.WordTiming) *video.ImageOverlay {
	wordIndex := findKeywordInTimings(timings, cue.Keyword)
	if wordIndex < 0 {
		slog.Debug("Keyword not found in timings", "keyword", cue.Keyword)
		return nil
	}

	results, err := f.imageSearch.Search(ctx, cue.SearchQuery, 5)
	if err != nil {
		slog.Debug("Image search failed", "query", cue.SearchQuery, "error", err)
		return nil
	}
	if len(results) == 0 {
		slog.Debug("No search results", "query", cue.SearchQuery)
		return nil
	}

	imageData, ext := f.downloadValid(ctx, results)
	if imageData == nil {
		return nil
	}

	imagePath := imagePath(imageDir, index, ext)
	if err := os.WriteFile(imagePath, imageData, 0644); err != nil {
		return nil
	}

	return &video.ImageOverlay{
		ImagePath: imagePath,
		StartTime: timings[wordIndex].StartTime,
		EndTime:   timings[wordIndex].StartTime + f.cfg.DisplayTime,
		Width:     f.cfg.ImageWidth,
		Height:    f.cfg.ImageHeight,
	}
}

func (f *Fetcher) downloadValid(ctx context.Context, results []imagesearch.SearchResult) ([]byte, string) {
	for i, result := range results {
		slog.Debug("Trying to download image", "index", i, "url", result.ImageURL)
		data, err := f.imageSearch.DownloadImage(ctx, result.ImageURL)
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

func (f *Fetcher) generateCues(ctx context.Context, script string) []llm.VisualCue {
	visuals, err := f.llm.GenerateVisualsForScript(ctx, script)
	if err != nil {
		return nil
	}
	return visuals
}

func (f *Fetcher) enforceConstraints(overlays []video.ImageOverlay) []video.ImageOverlay {
	if len(overlays) <= 1 {
		return overlays
	}

	filtered := make([]video.ImageOverlay, 0, len(overlays))
	filtered = append(filtered, overlays[0])

	for i := 1; i < len(overlays); i++ {
		gap := overlays[i].StartTime - filtered[len(filtered)-1].EndTime
		if gap >= f.cfg.MinGap {
			filtered = append(filtered, overlays[i])
		}
	}
	return filtered
}
