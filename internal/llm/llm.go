package llm

import "context"

type VisualCue struct {
	Keyword     string `json:"keyword"`
	SearchQuery string `json:"search_query"`
}

type ScriptWithVisuals struct {
	Script  string      `json:"script"`
	Visuals []VisualCue `json:"visuals"`
}

type Client interface {
	GenerateScript(ctx context.Context, topic string, scriptLength, hookDuration int) (string, error)
	GenerateConversation(ctx context.Context, topic string, speakers []string, scriptLength, hookDuration int) (string, error)
	GenerateScriptWithVisuals(ctx context.Context, topic string, scriptLength, hookDuration int) (*ScriptWithVisuals, error)
	GenerateVisualsForScript(ctx context.Context, script string) ([]VisualCue, error)
	GenerateTitle(ctx context.Context, script string) (string, error)
}
