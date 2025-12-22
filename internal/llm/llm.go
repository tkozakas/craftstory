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

type RedditThread struct {
	Title    string
	Post     string
	Comments []string
}

type Client interface {
	GenerateScript(ctx context.Context, topic string, scriptLength, hookDuration int) (string, error)
	GenerateConversation(ctx context.Context, topic string, speakers []string, scriptLength, hookDuration int) (string, error)
	GenerateRedditConversation(ctx context.Context, thread RedditThread, speakers []string, scriptLength, hookDuration int) (string, error)
	GenerateScriptWithVisuals(ctx context.Context, topic string, scriptLength, hookDuration int) (*ScriptWithVisuals, error)
	GenerateVisualsForScript(ctx context.Context, script string) ([]VisualCue, error)
	GenerateTitle(ctx context.Context, script string) (string, error)
}
