package search

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"path/filepath"
	"strings"

	"craftstory/internal/llm"
	"craftstory/internal/search/google"
	"craftstory/internal/search/tenor"
	"craftstory/internal/speech"
)

type VisualCue = llm.VisualCue

type ImageSearcher interface {
	Search(ctx context.Context, query string, count int) ([]google.Result, error)
	DownloadImage(ctx context.Context, imageURL string) ([]byte, error)
}

type GIFSearcher interface {
	Search(ctx context.Context, query string, limit int) ([]tenor.GIF, error)
	Download(ctx context.Context, gifURL string) ([]byte, error)
}

func findKeywordInTimings(timings []speech.WordTiming, keyword string, startFrom int) int {
	if keyword == "" || len(timings) == 0 {
		return -1
	}
	if startFrom < 0 {
		startFrom = 0
	}

	keywordLower := strings.ToLower(keyword)
	keywordWords := strings.Fields(keywordLower)

	if len(keywordWords) == 1 {
		for i := startFrom; i < len(timings); i++ {
			if cleanWord(timings[i].Word) == keywordLower {
				return i
			}
		}
		for i := startFrom; i < len(timings); i++ {
			cleaned := cleanWord(timings[i].Word)
			if strings.Contains(cleaned, keywordLower) || strings.Contains(keywordLower, cleaned) {
				return i
			}
		}
		for i := startFrom; i < len(timings); i++ {
			cleaned := cleanWord(timings[i].Word)
			if len(cleaned) > 3 && len(keywordLower) > 3 {
				if strings.HasPrefix(cleaned, keywordLower[:len(keywordLower)-1]) ||
					strings.HasPrefix(keywordLower, cleaned[:len(cleaned)-1]) {
					return i
				}
			}
		}
		return -1
	}

	for i := startFrom; i <= len(timings)-len(keywordWords); i++ {
		match := true
		for j, kw := range keywordWords {
			if cleanWord(timings[i+j].Word) != kw {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}

	firstWord := keywordWords[0]
	for i := startFrom; i < len(timings); i++ {
		if cleanWord(timings[i].Word) == firstWord {
			return i
		}
	}

	return -1
}

func findSpeakerSegmentEnd(timings []speech.WordTiming, startIndex int) float64 {
	if startIndex < 0 || startIndex >= len(timings) {
		return 0
	}

	speaker := timings[startIndex].Speaker
	lastEndTime := timings[startIndex].EndTime

	for i := startIndex + 1; i < len(timings); i++ {
		if timings[i].Speaker != speaker && speaker != "" {
			break
		}
		lastEndTime = timings[i].EndTime
	}

	return lastEndTime
}

func cleanWord(word string) string {
	return strings.ToLower(strings.Trim(word, ".,!?;:'\"()[]{}"))
}

func imagePath(dir string, index int, ext string) string {
	return filepath.Join(dir, fmt.Sprintf("image_%d%s", index, ext))
}

func detectImageFormat(data []byte) string {
	if len(data) < 12 {
		return ""
	}
	if bytes.HasPrefix(data, []byte{0xFF, 0xD8, 0xFF}) {
		return ".jpg"
	}
	if bytes.HasPrefix(data, []byte{0x89, 0x50, 0x4E, 0x47}) {
		return ".png"
	}
	if bytes.HasPrefix(data, []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")) {
		return ".webp"
	}
	return ""
}

func isValidImage(data []byte) bool {
	if len(data) < 100 {
		return false
	}
	if detectImageFormat(data) != "" {
		return true
	}
	_, _, err := image.Decode(bytes.NewReader(data))
	return err == nil
}

func isValidGif(data []byte) bool {
	if len(data) < 100 {
		return false
	}
	return bytes.HasPrefix(data, []byte("GIF87a")) || bytes.HasPrefix(data, []byte("GIF89a"))
}
