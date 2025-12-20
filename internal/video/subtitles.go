package video

import (
	"fmt"
	"strings"
)

type Subtitle struct {
	Word      string
	StartTime float64
	EndTime   float64
}

type SubtitleGenerator struct {
	fontName string
	fontSize int
}

type SubtitleOptions struct {
	FontName string
	FontSize int
}

func NewSubtitleGenerator(opts SubtitleOptions) *SubtitleGenerator {
	return &SubtitleGenerator{
		fontName: opts.FontName,
		fontSize: opts.FontSize,
	}
}

func (g *SubtitleGenerator) Generate(text string, audioDuration float64) []Subtitle {
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}

	subtitles := make([]Subtitle, 0, len(words))
	timePerWord := audioDuration / float64(len(words))

	for i, word := range words {
		startTime := float64(i) * timePerWord
		endTime := startTime + timePerWord

		subtitles = append(subtitles, Subtitle{
			Word:      word,
			StartTime: startTime,
			EndTime:   endTime,
		})
	}

	return subtitles
}

func (g *SubtitleGenerator) ToASS(subtitles []Subtitle) string {
	var sb strings.Builder

	sb.WriteString("[Script Info]\n")
	sb.WriteString("Title: Generated Subtitles\n")
	sb.WriteString("ScriptType: v4.00+\n")
	sb.WriteString("PlayResX: 1080\n")
	sb.WriteString("PlayResY: 1920\n")
	sb.WriteString("\n")

	sb.WriteString("[V4+ Styles]\n")
	sb.WriteString("Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding\n")
	sb.WriteString(fmt.Sprintf("Style: Default,%s,%d,&H00FFFFFF,&H000000FF,&H00000000,&H80000000,1,0,0,0,100,100,0,0,1,3,0,2,10,10,50,1\n",
		g.fontName, g.fontSize))
	sb.WriteString("\n")

	sb.WriteString("[Events]\n")
	sb.WriteString("Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n")

	for _, sub := range subtitles {
		start := formatASSTime(sub.StartTime)
		end := formatASSTime(sub.EndTime)
		sb.WriteString(fmt.Sprintf("Dialogue: 0,%s,%s,Default,,0,0,0,,%s\n", start, end, sub.Word))
	}

	return sb.String()
}

func formatASSTime(seconds float64) string {
	hours := int(seconds) / 3600
	minutes := (int(seconds) % 3600) / 60
	secs := int(seconds) % 60
	centis := int((seconds - float64(int(seconds))) * 100)

	return fmt.Sprintf("%d:%02d:%02d.%02d", hours, minutes, secs, centis)
}
