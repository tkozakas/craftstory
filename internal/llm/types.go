package llm

import "context"

type VisualCue struct {
	Keyword     string `json:"keyword"`
	SearchQuery string `json:"search_query"`
	Type        string `json:"type"`
}

type Client interface {
	GenerateScript(ctx context.Context, topic string, wordCount int) (string, error)
	GenerateConversation(ctx context.Context, topic string, speakers []string, wordCount int) (string, error)
	GenerateVisuals(ctx context.Context, script string, count int) ([]VisualCue, error)
	GenerateTitle(ctx context.Context, script string) (string, error)
}
