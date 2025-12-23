package visuals

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"path/filepath"
	"strings"

	"craftstory/internal/tts"
)

func findKeywordInTimings(timings []tts.WordTiming, keyword string) int {
	if keyword == "" || len(timings) == 0 {
		return -1
	}

	keywordLower := strings.ToLower(keyword)
	keywordWords := strings.Fields(keywordLower)

	if len(keywordWords) == 1 {
		for i, t := range timings {
			if cleanWord(t.Word) == keywordLower {
				return i
			}
		}
		for i, t := range timings {
			if strings.Contains(cleanWord(t.Word), keywordLower) {
				return i
			}
		}
		return -1
	}

	for i := 0; i <= len(timings)-len(keywordWords); i++ {
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
	return -1
}

func findSpeakerSegmentEnd(timings []tts.WordTiming, startIndex int) float64 {
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
