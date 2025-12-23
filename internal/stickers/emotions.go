package stickers

import (
	"strings"
)

type Emotion string

const (
	EmotionConfused Emotion = "confused"
	EmotionThinking Emotion = "thinking"
	EmotionHappy    Emotion = "happy"
	EmotionShocked  Emotion = "shocked"
	EmotionExplain  Emotion = "explain"
	EmotionLaughing Emotion = "laughing"
	EmotionNeutral  Emotion = "neutral"
)

type EmotionMapping struct {
	Emotion    Emotion
	StickerIDs []int
}

var DefaultEmotionMappings = []EmotionMapping{
	{EmotionConfused, []int{1, 3, 12}},
	{EmotionThinking, []int{2, 5, 15}},
	{EmotionHappy, []int{4, 10, 20}},
	{EmotionShocked, []int{9, 13, 18}},
	{EmotionExplain, []int{16, 19, 21}},
	{EmotionLaughing, []int{22, 25}},
	{EmotionNeutral, []int{6, 7, 8}},
}

var emotionPatterns = map[Emotion][]string{
	EmotionConfused: {
		"wait", "what", "huh", "confused", "don't get", "don't understand",
		"why", "how come", "seriously", "really", "are you sure", "but",
	},
	EmotionThinking: {
		"hmm", "let me think", "actually", "so", "maybe", "could be",
		"i think", "probably", "might", "consider", "wonder",
	},
	EmotionHappy: {
		"yes", "awesome", "perfect", "great", "love", "amazing", "nice",
		"exactly", "that's it", "got it", "aha", "finally", "works",
	},
	EmotionShocked: {
		"no way", "what the", "mind blown", "insane", "crazy", "whoa",
		"holy", "wow", "unbelievable", "can't believe", "seriously",
	},
	EmotionExplain: {
		"so basically", "here's the thing", "let me explain", "the trick is",
		"check this", "look", "see", "imagine", "think of it",
	},
	EmotionLaughing: {
		"haha", "lol", "funny", "hilarious", "joke", "laugh", "lmao",
	},
}

func DetectEmotion(text string) Emotion {
	lower := strings.ToLower(text)

	maxScore := 0
	detected := EmotionNeutral

	for emotion, patterns := range emotionPatterns {
		score := 0
		for _, pattern := range patterns {
			if strings.Contains(lower, pattern) {
				score++
			}
		}
		if score > maxScore {
			maxScore = score
			detected = emotion
		}
	}

	if strings.HasSuffix(strings.TrimSpace(text), "?") && detected == EmotionNeutral {
		return EmotionConfused
	}

	if strings.HasSuffix(strings.TrimSpace(text), "!") && detected == EmotionNeutral {
		return EmotionHappy
	}

	return detected
}

func GetStickerForEmotion(emotion Emotion, stickerCount int, lineIndex int) int {
	for _, mapping := range DefaultEmotionMappings {
		if mapping.Emotion == emotion {
			validIDs := make([]int, 0)
			for _, id := range mapping.StickerIDs {
				if id <= stickerCount {
					validIDs = append(validIDs, id)
				}
			}
			if len(validIDs) > 0 {
				return validIDs[lineIndex%len(validIDs)]
			}
		}
	}
	return 0
}
