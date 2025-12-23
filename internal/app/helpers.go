package app

import (
	"bytes"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"math/rand"
	"regexp"
	"strings"

	"craftstory/internal/dialogue"
	"craftstory/internal/stickers"
	"craftstory/internal/tts"
	"craftstory/internal/video"
)

var wordSplitRegex = regexp.MustCompile(`\s+`)

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

func buildVoiceMap(voices []tts.VoiceConfig) map[string]tts.VoiceConfig {
	m := make(map[string]tts.VoiceConfig, len(voices))
	for _, v := range voices {
		m[v.Name] = v
	}
	return m
}

func buildSpeakerColors(voiceMap map[string]tts.VoiceConfig) map[string]string {
	colors := make(map[string]string, len(voiceMap))
	for name, voice := range voiceMap {
		if voice.SubtitleColor != "" {
			colors[name] = voice.SubtitleColor
		}
	}
	return colors
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

func randomInt(n int) int {
	if n <= 0 {
		return 0
	}
	return rand.Intn(n)
}

func buildStickerOverlays(lines []dialogue.Line, segments []video.SegmentInfo, stickerProviders map[string]*stickers.Provider) []video.StickerOverlay {
	if len(lines) != len(segments) {
		return nil
	}

	var overlays []video.StickerOverlay
	for i, line := range lines {
		provider, ok := stickerProviders[line.Speaker]
		if !ok || provider == nil {
			continue
		}

		stickerID := line.StickerID
		if stickerID == 0 {
			emotion := stickers.DetectEmotion(line.Text)
			stickerID = stickers.GetStickerForEmotion(emotion, provider.Count(), i)
		}

		if stickerID == 0 {
			continue
		}

		stickerPath := provider.Get(stickerID)
		if stickerPath == "" {
			continue
		}

		overlays = append(overlays, video.StickerOverlay{
			StickerPath: stickerPath,
			Speaker:     line.Speaker,
			StartTime:   segments[i].StartTime,
			EndTime:     segments[i].EndTime,
		})
	}
	return overlays
}
