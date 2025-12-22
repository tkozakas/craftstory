package prompts

import (
	"bytes"
	"fmt"
	"os"
	"text/template"

	"gopkg.in/yaml.v3"
)

const defaultPromptsPath = "prompts.yaml"

type Prompts struct {
	System SystemPrompts `yaml:"system"`
	Script ScriptPrompts `yaml:"script"`
	Title  TitlePrompts  `yaml:"title"`
}

type SystemPrompts struct {
	Default            string `yaml:"default"`
	Conversation       string `yaml:"conversation"`
	ConversationReddit string `yaml:"conversation_reddit"`
	Visuals            string `yaml:"visuals"`
	Title              string `yaml:"title"`
}

type ScriptPrompts struct {
	Single             string `yaml:"single"`
	Conversation       string `yaml:"conversation"`
	RedditConversation string `yaml:"reddit_conversation"`
	WithVisuals        string `yaml:"with_visuals"`
	VisualsOnly        string `yaml:"visuals_only"`
}

type TitlePrompts struct {
	Generate string `yaml:"generate"`
}

type ScriptParams struct {
	Topic        string
	ScriptLength int
	HookDuration int
}

type ConversationParams struct {
	Topic        string
	ScriptLength int
	HookDuration int
	SpeakerList  string
	FirstSpeaker string
	LastSpeaker  string
}

type RedditConversationParams struct {
	RedditTitle    string
	RedditPost     string
	RedditComments string
	ScriptLength   int
	HookDuration   int
	SpeakerList    string
	FirstSpeaker   string
	LastSpeaker    string
}

type VisualsParams struct {
	Topic        string
	ScriptLength int
	HookDuration int
}

type VisualsOnlyParams struct {
	Script string
}

type TitleParams struct {
	Script string
}

func Load() (*Prompts, error) {
	return LoadFrom(defaultPromptsPath)
}

func LoadFrom(path string) (*Prompts, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read prompts file: %w", err)
	}

	var p Prompts
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("failed to parse prompts file: %w", err)
	}

	return &p, nil
}

func (p *Prompts) RenderScript(params ScriptParams) (string, error) {
	return render(p.Script.Single, params)
}

func (p *Prompts) RenderConversation(params ConversationParams) (string, error) {
	return render(p.Script.Conversation, params)
}

func (p *Prompts) RenderRedditConversation(params RedditConversationParams) (string, error) {
	return render(p.Script.RedditConversation, params)
}

func (p *Prompts) RenderVisuals(params VisualsParams) (string, error) {
	return render(p.Script.WithVisuals, params)
}

func (p *Prompts) RenderVisualsOnly(params VisualsOnlyParams) (string, error) {
	return render(p.Script.VisualsOnly, params)
}

func (p *Prompts) RenderTitle(params TitleParams) (string, error) {
	return render(p.Title.Generate, params)
}

func render(tmpl string, data any) (string, error) {
	t, err := template.New("prompt").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}
