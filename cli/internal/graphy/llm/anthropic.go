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
const DefaultAnthropicModel = "claude-opus-5"

// anthropicMaxTokens bounds one plan response.
//
// It is not just the reply budget: Claude Opus 5 thinks by default (Opus 4.8,
// the previous default here, did not), and max_tokens caps thinking plus
// response text together. The old 2048 was sized for reply-only and would
// truncate the plan JSON mid-object once thinking started consuming it.
const anthropicMaxTokens = 4096

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
		"max_tokens": anthropicMaxTokens,
		"system":     system,
		"messages": []map[string]any{
			{"role": "user", "content": user},
		},
		// Picking seed nodes out of a supplied index is retrieval, not
		// reasoning, so the cheapest effort level is the right one — and it
		// keeps the thinking that Opus 5 does by default from dominating the
		// token budget above.
		"output_config": map[string]any{"effort": "low"},
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
