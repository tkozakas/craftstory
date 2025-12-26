package search

import (
	"context"
	"log/slog"
	"os"
	"sort"
	"strings"

	"craftstory/internal/speech"
	"craftstory/internal/video"
)

// FetcherConfig configures the visual fetcher.
type FetcherConfig struct {
	MaxDisplayTime float64
	ImageWidth     int
	ImageHeight    int
	MinGap         float64
}

// FetchRequest contains parameters for fetching visuals.
type FetchRequest struct {
	Script   string
	Visuals  []VisualCue
	Timings  []speech.WordTiming
	ImageDir string
}

// Fetcher fetches images and GIFs based on visual cues.
type Fetcher struct {
	imageSearch ImageSearcher
	gifSearch   GIFSearcher
	cfg         FetcherConfig
}

// NewFetcher creates a new visual fetcher.
func NewFetcher(imageSearch ImageSearcher, gifSearch GIFSearcher, cfg FetcherConfig) *Fetcher {
	return &Fetcher{
		imageSearch: imageSearch,
		gifSearch:   gifSearch,
		cfg:         cfg,
	}
}

// Fetch fetches visuals for the given request and returns image overlays.
func (f *Fetcher) Fetch(ctx context.Context, req FetchRequest) []video.ImageOverlay {
	if f.imageSearch == nil && f.gifSearch == nil {
		slog.Warn("No search clients configured")
		return nil
	}
	if len(req.Visuals) == 0 {
		slog.Warn("No visual cues provided")
		return nil
	}

	slog.Info("Processing visual cues", "count", len(req.Visuals), "timings_count", len(req.Timings))

	overlays := make([]video.ImageOverlay, 0, len(req.Visuals))

	for i, cue := range req.Visuals {
		slog.Info("Processing visual cue", "index", i, "keyword", cue.Keyword, "query", cue.SearchQuery)
		overlay := f.fetchSingle(ctx, req.ImageDir, i, cue, req.Timings)
		if overlay != nil {
			overlays = append(overlays, *overlay)
			slog.Info("Fetched visual", "keyword", cue.Keyword, "path", overlay.ImagePath, "start", overlay.StartTime, "end", overlay.EndTime)
		} else {
			slog.Warn("Failed to fetch visual", "keyword", cue.Keyword, "query", cue.SearchQuery)
		}
	}

	sort.Slice(overlays, func(i, j int) bool {
		return overlays[i].StartTime < overlays[j].StartTime
	})

	slog.Info("Visual fetch complete", "requested", len(req.Visuals), "success", len(overlays))
	return f.enforceConstraints(overlays)
}

func (f *Fetcher) fetchSingle(ctx context.Context, imageDir string, index int, cue VisualCue, timings []speech.WordTiming) *video.ImageOverlay {
	wordIndex := findKeywordInTimings(timings, cue.Keyword)
	if wordIndex < 0 {
		slog.Warn("Keyword not found in timings", "keyword", cue.Keyword)
		return nil
	}
	slog.Info("Found keyword in timings", "keyword", cue.Keyword, "word_index", wordIndex, "time", timings[wordIndex].StartTime)

	isGif := cue.Type == "gif"

	var imageData []byte
	var ext string

	if isGif {
		imageData, ext = f.fetchGIF(ctx, cue.SearchQuery)
	} else {
		imageData, ext = f.fetchImage(ctx, cue.SearchQuery)
	}

	if imageData == nil {
		return nil
	}

	filePath := imagePath(imageDir, index, ext)
	if err := os.WriteFile(filePath, imageData, 0644); err != nil {
		slog.Warn("Failed to write file", "path", filePath, "error", err)
		return nil
	}

	startTime := timings[wordIndex].StartTime
	segmentEnd := findSpeakerSegmentEnd(timings, wordIndex)

	endTime := segmentEnd
	if f.cfg.MaxDisplayTime > 0 && (segmentEnd-startTime) > f.cfg.MaxDisplayTime {
		endTime = startTime + f.cfg.MaxDisplayTime
	}

	return &video.ImageOverlay{
		ImagePath: filePath,
		StartTime: startTime,
		EndTime:   endTime,
		Width:     f.cfg.ImageWidth,
		Height:    f.cfg.ImageHeight,
		IsGif:     isGif,
	}
}

func (f *Fetcher) fetchGIF(ctx context.Context, query string) ([]byte, string) {
	if f.gifSearch == nil {
		slog.Debug("GIF search not configured")
		return nil, ""
	}

	gifs, err := f.gifSearch.Search(ctx, query, 5)
	if err != nil {
		slog.Warn("GIF search failed", "query", query, "error", err)
		return nil, ""
	}
	if len(gifs) == 0 {
		slog.Warn("No GIFs found", "query", query)
		return nil, ""
	}

	for _, gif := range gifs {
		data, err := f.gifSearch.Download(ctx, gif.URL)
		if err != nil {
			slog.Debug("GIF download failed", "url", gif.URL, "error", err)
			continue
		}
		if !isValidGif(data) {
			slog.Debug("Invalid GIF format", "size", len(data))
			continue
		}
		if len(data) < 5000 {
			slog.Debug("GIF too small", "size", len(data))
			continue
		}
		return data, ".gif"
	}

	return nil, ""
}

func (f *Fetcher) fetchImage(ctx context.Context, query string) ([]byte, string) {
	if f.imageSearch == nil {
		slog.Debug("Image search not configured")
		return nil, ""
	}

	results, err := f.imageSearch.Search(ctx, query, 5)
	if err != nil {
		slog.Warn("Image search failed", "query", query, "error", err)
		return nil, ""
	}
	if len(results) == 0 {
		slog.Warn("No images found", "query", query)
		return nil, ""
	}

	for _, result := range results {
		data, err := f.imageSearch.DownloadImage(ctx, result.ImageURL)
		if err != nil {
			slog.Debug("Image download failed", "url", result.ImageURL, "error", err)
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
		return data, ext
	}

	return nil, ""
}

func (f *Fetcher) enforceConstraints(overlays []video.ImageOverlay) []video.ImageOverlay {
	if len(overlays) <= 1 {
		return overlays
	}

	for i := 1; i < len(overlays); i++ {
		prevEnd := overlays[i-1].EndTime
		currStart := overlays[i].StartTime

		if currStart < prevEnd+f.cfg.MinGap {
			newEnd := currStart - f.cfg.MinGap
			if newEnd < overlays[i-1].StartTime+0.5 {
				newEnd = overlays[i-1].StartTime + 0.5
			}
			slog.Debug("Truncating overlay", "index", i-1, "old_end", prevEnd, "new_end", newEnd)
			overlays[i-1].EndTime = newEnd
		}
	}

	for i, o := range overlays {
		slog.Info("Final overlay", "index", i, "path", o.ImagePath, "start", o.StartTime, "end", o.EndTime)
	}

	return overlays
}
