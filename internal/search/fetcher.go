package search

import (
	"context"
	"log/slog"
	"os"

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
	lastWordIndex := 0

	for i, cue := range req.Visuals {
		slog.Info("Processing visual cue", "index", i, "keyword", cue.Keyword, "query", cue.SearchQuery, "start_from", lastWordIndex)
		overlay, wordIndex := f.fetchSingle(ctx, req.ImageDir, i, cue, req.Timings, lastWordIndex)
		if overlay != nil {
			overlays = append(overlays, *overlay)
			lastWordIndex = wordIndex + 1
			slog.Info("Fetched visual", "keyword", cue.Keyword, "path", overlay.ImagePath, "start", overlay.StartTime, "end", overlay.EndTime, "word_index", wordIndex)
		} else {
			slog.Warn("Failed to fetch visual", "keyword", cue.Keyword, "query", cue.SearchQuery)
		}
	}

	slog.Info("Visual fetch complete", "requested", len(req.Visuals), "success", len(overlays))
	return f.enforceConstraints(overlays)
}

func (f *Fetcher) fetchSingle(ctx context.Context, imageDir string, index int, cue VisualCue, timings []speech.WordTiming, startFrom int) (*video.ImageOverlay, int) {
	wordIndex := findKeywordInTimings(timings, cue.Keyword, startFrom)
	if wordIndex < 0 && startFrom > 0 {
		slog.Debug("Keyword not found after position, trying from start", "keyword", cue.Keyword, "start_from", startFrom)
		wordIndex = findKeywordInTimings(timings, cue.Keyword, 0)
	}
	if wordIndex < 0 {
		slog.Warn("Keyword not found in timings", "keyword", cue.Keyword)
		return nil, -1
	}
	slog.Info("Found keyword in timings", "keyword", cue.Keyword, "word_index", wordIndex, "time", timings[wordIndex].StartTime)

	isGif := cue.Type == "gif" && f.gifSearch != nil

	var imageData []byte
	var ext string

	if isGif {
		imageData, ext = f.fetchGIF(ctx, cue.SearchQuery)
	} else {
		imageData, ext = f.fetchImage(ctx, cue.SearchQuery)
	}

	if imageData == nil {
		return nil, -1
	}

	filePath := imagePath(imageDir, index, ext)
	if err := os.WriteFile(filePath, imageData, 0644); err != nil {
		slog.Warn("Failed to write file", "path", filePath, "error", err)
		return nil, -1
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
	}, wordIndex
}

func (f *Fetcher) fetchGIF(ctx context.Context, query string) ([]byte, string) {
	if f.gifSearch == nil {
		slog.Debug("GIF search not configured")
		return nil, ""
	}

	slog.Info("[Tenor] Searching", "query", query)
	gifs, err := f.gifSearch.Search(ctx, query, 5)
	if err != nil {
		slog.Warn("[Tenor] Search failed", "query", query, "error", err)
		return nil, ""
	}
	if len(gifs) == 0 {
		slog.Warn("[Tenor] No results", "query", query)
		return nil, ""
	}
	slog.Info("[Tenor] Found results", "query", query, "count", len(gifs))

	for i, gif := range gifs {
		slog.Debug("[Tenor] Trying result", "index", i, "url", gif.URL)
		data, err := f.gifSearch.Download(ctx, gif.URL)
		if err != nil {
			slog.Debug("[Tenor] Download failed", "url", gif.URL, "error", err)
			continue
		}
		if !isValidGif(data) {
			slog.Debug("[Tenor] Invalid format", "size", len(data))
			continue
		}
		if len(data) < 5000 {
			slog.Debug("[Tenor] Too small", "size", len(data))
			continue
		}
		slog.Info("[Tenor] Downloaded", "query", query, "size", len(data))
		return data, ".gif"
	}

	slog.Warn("[Tenor] All downloads failed", "query", query)
	return nil, ""
}

func (f *Fetcher) fetchImage(ctx context.Context, query string) ([]byte, string) {
	if f.imageSearch == nil {
		slog.Debug("Image search not configured")
		return nil, ""
	}

	slog.Info("[Google] Searching", "query", query)
	results, err := f.imageSearch.Search(ctx, query, 5)
	if err != nil {
		slog.Warn("[Google] Search failed", "query", query, "error", err)
		return nil, ""
	}
	if len(results) == 0 {
		slog.Warn("[Google] No results", "query", query)
		return nil, ""
	}
	slog.Info("[Google] Found results", "query", query, "count", len(results))

	for i, result := range results {
		slog.Debug("[Google] Trying result", "index", i, "url", result.ImageURL)
		data, err := f.imageSearch.DownloadImage(ctx, result.ImageURL)
		if err != nil {
			slog.Debug("[Google] Download failed", "url", result.ImageURL, "error", err)
			continue
		}
		if !isValidImage(data) {
			slog.Debug("[Google] Invalid format", "size", len(data))
			continue
		}
		if len(data) < 10000 {
			slog.Debug("[Google] Too small", "size", len(data))
			continue
		}

		ext := detectImageFormat(data)
		if ext == "" {
			ext = ".jpg"
		}
		slog.Info("[Google] Downloaded", "query", query, "size", len(data), "format", ext)
		return data, ext
	}

	slog.Warn("[Google] All downloads failed", "query", query)
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
