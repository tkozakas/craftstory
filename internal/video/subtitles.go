package video

import (
	"fmt"
	"strings"

	"craftstory/internal/tts"
)

type Subtitle struct {
	Word      string
	StartTime float64
	EndTime   float64
}

type SubtitleGenerator struct {
	fontName     string
	fontSize     int
	primaryColor string
	outlineColor string
	outlineSize  int
	shadowSize   int
	bold         bool
}

type SubtitleOptions struct {
	FontName     string
	FontSize     int
	PrimaryColor string
	OutlineColor string
	OutlineSize  int
	ShadowSize   int
	Bold         bool
}

func NewSubtitleGenerator(opts SubtitleOptions) *SubtitleGenerator {
	primaryColor := "&H00FFFFFF" // white default
	if opts.PrimaryColor != "" {
		primaryColor = toASSColor(opts.PrimaryColor)
	}

	outlineColor := "&H00000000" // black default
	if opts.OutlineColor != "" {
		outlineColor = toASSColor(opts.OutlineColor)
	}

	outlineSize := 4
	if opts.OutlineSize > 0 {
		outlineSize = opts.OutlineSize
	}

	shadowSize := 2
	if opts.ShadowSize >= 0 {
		shadowSize = opts.ShadowSize
	}

	return &SubtitleGenerator{
		fontName:     opts.FontName,
		fontSize:     opts.FontSize,
		primaryColor: primaryColor,
		outlineColor: outlineColor,
		outlineSize:  outlineSize,
		shadowSize:   shadowSize,
		bold:         opts.Bold,
	}
}

func toASSColor(color string) string {
	if strings.HasPrefix(color, "&H") {
		return color
	}
	color = strings.TrimPrefix(color, "#")
	if len(color) == 6 {
		r := color[0:2]
		g := color[2:4]
		b := color[4:6]
		return fmt.Sprintf("&H00%s%s%s", b, g, r)
	}
	return "&H00FFFFFF"
}

func (g *SubtitleGenerator) GenerateFromTimings(timings []tts.WordTiming) []Subtitle {
	subtitles := make([]Subtitle, 0, len(timings))
	for _, t := range timings {
		subtitles = append(subtitles, Subtitle{
			Word:      t.Word,
			StartTime: t.StartTime,
			EndTime:   t.EndTime,
		})
	}
	return subtitles
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

	boldVal := 0
	if g.bold {
		boldVal = -1
	}

	sb.WriteString("[V4+ Styles]\n")
	sb.WriteString("Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding\n")
	sb.WriteString(fmt.Sprintf("Style: Default,%s,%d,%s,%s,%s,&H80000000,%d,0,0,0,100,100,0,0,1,%d,%d,5,10,10,50,1\n",
		g.fontName, g.fontSize, g.primaryColor, g.primaryColor, g.outlineColor, boldVal, g.outlineSize, g.shadowSize))
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
