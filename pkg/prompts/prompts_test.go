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
  single: "Generate {{.WordCount}} word script about {{.Topic}}"
  conversation: "Conversation about {{.Topic}} with {{.SpeakerList}}"
  visuals: "Extract visuals from: {{.Script}}"

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
			Single: "Generate {{.WordCount}} word script about {{.Topic}}",
		},
	}

	result, err := p.RenderScript(ScriptParams{
		Topic:     "space",
		WordCount: 150,
	})
	if err != nil {
		t.Fatalf("RenderScript() error = %v", err)
	}

	expected := "Generate 150 word script about space"
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
		WordCount:    150,
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
			Visuals: "Extract visuals from: {{.Script}}",
		},
	}

	result, err := p.RenderVisuals(VisualsParams{
		Script: "A story about nature and trees",
	})
	if err != nil {
		t.Fatalf("RenderVisuals() error = %v", err)
	}

	expected := "Extract visuals from: A story about nature and trees"
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
