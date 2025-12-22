package dialogue

import (
	"regexp"
	"strings"
)

type Line struct {
	Speaker string
	Text    string
}

type Script struct {
	Lines []Line
}

var linePattern = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9 ]*?)\s*:\s*(.+)$`)

func Parse(text string) *Script {
	lines := strings.Split(text, "\n")
	script := &Script{
		Lines: make([]Line, 0),
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "(") || strings.HasPrefix(line, "[") {
			continue
		}

		matches := linePattern.FindStringSubmatch(line)
		if len(matches) == 3 {
			speaker := strings.TrimSpace(matches[1])
			text := strings.TrimSpace(matches[2])
			if strings.HasPrefix(text, "(") && strings.HasSuffix(text, ")") {
				continue
			}
			text = stripFormatting(text)
			script.Lines = append(script.Lines, Line{
				Speaker: speaker,
				Text:    text,
			})
		}
	}

	return script
}

func stripFormatting(text string) string {
	text = strings.ReplaceAll(text, "*", "")
	text = strings.ReplaceAll(text, "_", "")
	text = strings.ReplaceAll(text, "~", "")
	return text
}

func (s *Script) Speakers() []string {
	seen := make(map[string]bool)
	speakers := make([]string, 0)

	for _, line := range s.Lines {
		if !seen[line.Speaker] {
			seen[line.Speaker] = true
			speakers = append(speakers, line.Speaker)
		}
	}

	return speakers
}

func (s *Script) IsEmpty() bool {
	return len(s.Lines) == 0
}

func (s *Script) FullText() string {
	var texts []string
	for _, line := range s.Lines {
		texts = append(texts, line.Text)
	}
	return strings.Join(texts, " ")
}
