package visuals

import (
	"context"
	"log/slog"
	"os"
	"sort"
	"strings"

	"craftstory/internal/llm"
	"craftstory/internal/tts"
	"craftstory/internal/video"
)

type ImageSearcher interface {
	Search(ctx context.Context, query string, count int) ([]SearchResult, error)
	SearchGif(ctx context.Context, query string, count int) ([]SearchResult, error)
	DownloadImage(ctx context.Context, imageURL string) ([]byte, error)
}

type Config struct {
	MaxDisplayTime float64
	ImageWidth     int
	ImageHeight    int
	MinGap         float64
}

type FetchRequest struct {
	Script   string
	Visuals  []llm.VisualCue
	Timings  []tts.WordTiming
	ImageDir string
}

type Fetcher struct {
	imageSearch ImageSearcher
	cfg         Config
}

func NewFetcher(imageSearch ImageSearcher, cfg Config) *Fetcher {
	return &Fetcher{
		imageSearch: imageSearch,
		cfg:         cfg,
	}
}

func (f *Fetcher) Fetch(ctx context.Context, req FetchRequest) []video.ImageOverlay {
	if f.imageSearch == nil {
		slog.Warn("Image search client is nil")
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
			slog.Info("Successfully fetched image", "keyword", cue.Keyword, "path", overlay.ImagePath, "start", overlay.StartTime, "end", overlay.EndTime)
		} else {
			slog.Warn("Failed to fetch image for cue", "keyword", cue.Keyword, "query", cue.SearchQuery)
		}
	}

	// Sort overlays by start time before enforcing constraints
	sort.Slice(overlays, func(i, j int) bool {
		return overlays[i].StartTime < overlays[j].StartTime
	})

	slog.Info("Image fetch complete", "requested", len(req.Visuals), "success", len(overlays))
	return f.enforceConstraints(overlays)
}

func (f *Fetcher) fetchSingle(ctx context.Context, imageDir string, index int, cue llm.VisualCue, timings []tts.WordTiming) *video.ImageOverlay {
	wordIndex := findKeywordInTimings(timings, cue.Keyword)
	if wordIndex < 0 {
		slog.Warn("Keyword not found in timings", "keyword", cue.Keyword)
		return nil
	}
	slog.Info("Found keyword in timings", "keyword", cue.Keyword, "word_index", wordIndex, "time", timings[wordIndex].StartTime)

	isGif := cue.Type == "gif"
	var results []SearchResult
	var err error

	if isGif {
		results, err = f.imageSearch.SearchGif(ctx, cue.SearchQuery, 5)
	} else {
		results, err = f.imageSearch.Search(ctx, cue.SearchQuery, 5)
	}

	if err != nil {
		slog.Warn("Search failed", "query", cue.SearchQuery, "type", cue.Type, "error", err)
		return nil
	}
	if len(results) == 0 {
		slog.Warn("No search results returned", "query", cue.SearchQuery)
		return nil
	}
	slog.Info("Got search results", "query", cue.SearchQuery, "type", cue.Type, "count", len(results))

	imageData, ext := f.downloadValid(ctx, results, isGif)
	if imageData == nil {
		slog.Warn("Failed to download any valid media", "query", cue.SearchQuery)
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

func (f *Fetcher) downloadValid(ctx context.Context, results []SearchResult, isGif bool) ([]byte, string) {
	for i, result := range results {
		slog.Debug("Trying to download", "index", i, "url", result.ImageURL)
		data, err := f.imageSearch.DownloadImage(ctx, result.ImageURL)
		if err != nil {
			slog.Debug("Download failed", "error", err)
			continue
		}

		if isGif {
			if !isValidGif(data) {
				slog.Debug("Invalid GIF format", "size", len(data))
				continue
			}
			if len(data) < 5000 {
				slog.Debug("GIF too small", "size", len(data))
				continue
			}
			slog.Debug("GIF downloaded successfully", "size", len(data))
			return data, ".gif"
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
