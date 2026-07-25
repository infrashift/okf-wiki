package llm

import (
	"context"
	"encoding/json"
	"fmt"
)

// Anthropic calls the Claude Messages API.
type Anthropic struct {
	Key   string
	Model string // "" → DefaultAnthropicModel
	Base  string // "" → https://api.anthropic.com
	HTTP  httpDoer
}

// DefaultAnthropicModel is the current recommended Claude model alias.
const DefaultAnthropicModel = "claude-opus-4-8"

func (a *Anthropic) Name() string { return "anthropic" }

func (a *Anthropic) Complete(ctx context.Context, system, user string) (string, error) {
	base := a.Base
	if base == "" {
		base = "https://api.anthropic.com"
	}
	model := a.Model
	if model == "" {
		model = DefaultAnthropicModel
	}
	payload := map[string]any{
		"model":      model,
		"max_tokens": 2048,
		"system":     system,
		"messages": []map[string]any{
			{"role": "user", "content": user},
		},
	}
	headers := map[string]string{
		"x-api-key":         a.Key,
		"anthropic-version": "2023-06-01",
	}
	data, err := postJSON(ctx, asClient(a.HTTP), base+"/v1/messages", headers, payload)
	if err != nil {
		return "", err
	}
	var out struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", fmt.Errorf("anthropic response: %w", err)
	}
	for _, c := range out.Content {
		if c.Type == "text" && c.Text != "" {
			return c.Text, nil
		}
	}
	return "", fmt.Errorf("anthropic response had no text content")
}
