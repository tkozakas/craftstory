package app

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type session struct {
	id      string
	dir     string
	baseDir string
}

var sanitizeRegex = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

func newSession(baseDir string) *session {
	return &session{
		id:      time.Now().Format("20060102_150405"),
		baseDir: baseDir,
	}
}

func (s *session) finalize(title string) error {
	sanitized := sanitizeForPath(title)
	if sanitized == "" {
		sanitized = "untitled"
	}
	if len(sanitized) > 50 {
		sanitized = sanitized[:50]
	}

	s.dir = filepath.Join(s.baseDir, fmt.Sprintf("%s_%s", s.id, sanitized))
	return os.MkdirAll(s.dir, 0755)
}

func (s *session) audioPath() string  { return filepath.Join(s.dir, "audio.mp3") }
func (s *session) videoPath() string  { return filepath.Join(s.dir, "video.mp4") }
func (s *session) scriptPath() string { return filepath.Join(s.dir, "script.txt") }

func sanitizeForPath(s string) string {
	s = strings.ToLower(s)
	s = sanitizeRegex.ReplaceAllString(s, "_")
	return strings.Trim(s, "_")
}
