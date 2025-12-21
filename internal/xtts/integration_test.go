//go:build integration

package xtts

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func getServerURL() string {
	if url := os.Getenv("XTTS_SERVER_URL"); url != "" {
		return url
	}
	return "http://localhost:8020"
}

func getSpeaker() string {
	if s := os.Getenv("XTTS_SPEAKER"); s != "" {
		return s
	}
	return "default"
}

func skipIfNoServer(t *testing.T, c *Client) {
	t.Helper()
	if !c.IsServerRunning() {
		t.Skip("XTTS server not running")
	}
}

func TestIntegrationHealth(t *testing.T) {
	c := NewClient(Options{ServerURL: getServerURL()})
	skipIfNoServer(t, c)

	if !c.IsServerRunning() {
		t.Error("health check failed")
	}
}

func TestIntegrationGenerate(t *testing.T) {
	c := NewClient(Options{
		ServerURL: getServerURL(),
		Speaker:   getSpeaker(),
		Language:  "en",
	})
	skipIfNoServer(t, c)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := c.GenerateSpeechWithTimings(ctx, "Hello, this is a test.")
	if err != nil {
		t.Fatalf("error = %v", err)
	}

	if len(result.Audio) == 0 {
		t.Error("audio empty")
	}

	if len(result.Timings) == 0 {
		t.Error("timings empty")
	}

	t.Logf("generated %d bytes, %d words", len(result.Audio), len(result.Timings))
}

func TestIntegrationSaveFile(t *testing.T) {
	c := NewClient(Options{
		ServerURL: getServerURL(),
		Speaker:   getSpeaker(),
		Language:  "en",
	})
	skipIfNoServer(t, c)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	audio, err := c.GenerateSpeech(ctx, "Save this to a file.")
	if err != nil {
		t.Fatalf("error = %v", err)
	}

	outPath := filepath.Join(t.TempDir(), "output.wav")
	if err := os.WriteFile(outPath, audio, 0644); err != nil {
		t.Fatalf("write error = %v", err)
	}

	info, _ := os.Stat(outPath)
	if info.Size() == 0 {
		t.Error("file empty")
	}

	t.Logf("saved %d bytes", info.Size())
}

func TestIntegrationLongText(t *testing.T) {
	c := NewClient(Options{
		ServerURL: getServerURL(),
		Speaker:   getSpeaker(),
		Language:  "en",
	})
	skipIfNoServer(t, c)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	text := `The ocean covers more than seventy percent of Earth's surface.
	Deep beneath the waves live creatures we have never seen.
	Some glow in the dark. Others have no eyes at all.`

	result, err := c.GenerateSpeechWithTimings(ctx, text)
	if err != nil {
		t.Fatalf("error = %v", err)
	}

	if len(result.Audio) == 0 {
		t.Error("audio empty")
	}

	if len(result.Timings) < 10 {
		t.Errorf("timings = %d, want >= 10", len(result.Timings))
	}

	t.Logf("generated %d bytes, %d words", len(result.Audio), len(result.Timings))
}
