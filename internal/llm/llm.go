package llm

import (
	"context"
)

// Client is the interface for LLM integration.
// When LLM is disabled, use NoopClient which returns empty strings.
type Client interface {
	Analyze(ctx context.Context, prompt string, codeContext string) (string, error)
}

type noopClient struct{}

func NewNoopClient() Client {
	return &noopClient{}
}

func (n *noopClient) Analyze(ctx context.Context, prompt string, codeContext string) (string, error) {
	return "", nil
}
