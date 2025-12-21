package prompts

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	tmpDir := t.TempDir()
	originalWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(originalWd) }()

	promptsContent := `
system:
  default: "Default system prompt"
  conversation: "Conversation system prompt"
  visuals: "Visuals system prompt"
  title: "Title system prompt"

script:
  single: "Generate {{.ScriptLength}}-second script about {{.Topic}}"
  conversation: "Conversation about {{.Topic}} with {{.SpeakerList}}"
  with_visuals: "Script with {{.MaxVisuals}} visuals about {{.Topic}}"

title:
  generate: "Title for: {{.Script}}"
`
	if err := os.WriteFile(filepath.Join(tmpDir, "prompts.yaml"), []byte(promptsContent), 0644); err != nil {
		t.Fatal(err)
	}

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	p, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if p.System.Default != "Default system prompt" {
		t.Errorf("System.Default = %q, want %q", p.System.Default, "Default system prompt")
	}
	if p.System.Conversation != "Conversation system prompt" {
		t.Errorf("System.Conversation = %q, want %q", p.System.Conversation, "Conversation system prompt")
	}
}

func TestLoadFrom(t *testing.T) {
	tmpDir := t.TempDir()
	promptsPath := filepath.Join(tmpDir, "custom.yaml")

	promptsContent := `
system:
  default: "Custom default"
script:
  single: "Custom script"
`
	if err := os.WriteFile(promptsPath, []byte(promptsContent), 0644); err != nil {
		t.Fatal(err)
	}

	p, err := LoadFrom(promptsPath)
	if err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
	}

	if p.System.Default != "Custom default" {
		t.Errorf("System.Default = %q, want %q", p.System.Default, "Custom default")
	}
}

func TestLoadFromMissing(t *testing.T) {
	_, err := LoadFrom("/nonexistent/path.yaml")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestLoadFromInvalid(t *testing.T) {
	tmpDir := t.TempDir()
	promptsPath := filepath.Join(tmpDir, "invalid.yaml")

	if err := os.WriteFile(promptsPath, []byte("not: valid: yaml: content:"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadFrom(promptsPath)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestRenderScript(t *testing.T) {
	p := &Prompts{
		Script: ScriptPrompts{
			Single: "Generate {{.ScriptLength}}-second script about {{.Topic}} with {{.HookDuration}}s hook",
		},
	}

	result, err := p.RenderScript(ScriptParams{
		Topic:        "space",
		ScriptLength: 60,
		HookDuration: 3,
	})
	if err != nil {
		t.Fatalf("RenderScript() error = %v", err)
	}

	expected := "Generate 60-second script about space with 3s hook"
	if result != expected {
		t.Errorf("RenderScript() = %q, want %q", result, expected)
	}
}

func TestRenderConversation(t *testing.T) {
	p := &Prompts{
		Script: ScriptPrompts{
			Conversation: "{{.FirstSpeaker}} and {{.LastSpeaker}} discuss {{.Topic}}",
		},
	}

	result, err := p.RenderConversation(ConversationParams{
		Topic:        "history",
		FirstSpeaker: "Host",
		LastSpeaker:  "Guest",
		SpeakerList:  "Host, Guest",
		ScriptLength: 60,
		HookDuration: 3,
	})
	if err != nil {
		t.Fatalf("RenderConversation() error = %v", err)
	}

	expected := "Host and Guest discuss history"
	if result != expected {
		t.Errorf("RenderConversation() = %q, want %q", result, expected)
	}
}

func TestRenderVisuals(t *testing.T) {
	p := &Prompts{
		Script: ScriptPrompts{
			WithVisuals: "Script about {{.Topic}} for {{.ScriptLength}} seconds",
		},
	}

	result, err := p.RenderVisuals(VisualsParams{
		Topic:        "nature",
		ScriptLength: 60,
		HookDuration: 3,
	})
	if err != nil {
		t.Fatalf("RenderVisuals() error = %v", err)
	}

	expected := "Script about nature for 60 seconds"
	if result != expected {
		t.Errorf("RenderVisuals() = %q, want %q", result, expected)
	}
}

func TestRenderTitle(t *testing.T) {
	p := &Prompts{
		Title: TitlePrompts{
			Generate: "Title for: {{.Script}}",
		},
	}

	result, err := p.RenderTitle(TitleParams{Script: "A story about space"})
	if err != nil {
		t.Fatalf("RenderTitle() error = %v", err)
	}

	expected := "Title for: A story about space"
	if result != expected {
		t.Errorf("RenderTitle() = %q, want %q", result, expected)
	}
}

func TestRenderInvalidTemplate(t *testing.T) {
	p := &Prompts{
		Script: ScriptPrompts{
			Single: "{{.Invalid",
		},
	}

	_, err := p.RenderScript(ScriptParams{Topic: "test"})
	if err == nil {
		t.Error("expected error for invalid template")
	}
}
