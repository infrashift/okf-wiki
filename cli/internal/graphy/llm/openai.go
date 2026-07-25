package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// httpDoer lets tests inject a client; nil falls back to a default client.
type httpDoer = *http.Client

func asClient(c httpDoer) *http.Client {
	if c != nil {
		return c
	}
	return http.DefaultClient
}

// OpenAI calls the Chat Completions API.
type OpenAI struct {
	Key   string
	Model string // "" → DefaultOpenAIModel
	Base  string // "" → https://api.openai.com
	HTTP  httpDoer
}

// DefaultOpenAIModel is the default OpenAI model.
const DefaultOpenAIModel = "gpt-4o-mini"

func (o *OpenAI) Name() string { return "openai" }

func (o *OpenAI) Complete(ctx context.Context, system, user string) (string, error) {
	base := o.Base
	if base == "" {
		base = "https://api.openai.com"
	}
	model := o.Model
	if model == "" {
		model = DefaultOpenAIModel
	}
	payload := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
	}
	headers := map[string]string{"Authorization": "Bearer " + o.Key}
	data, err := postJSON(ctx, asClient(o.HTTP), base+"/v1/chat/completions", headers, payload)
	if err != nil {
		return "", err
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", fmt.Errorf("openai response: %w", err)
	}
	for _, c := range out.Choices {
		if c.Message.Content != "" {
			return c.Message.Content, nil
		}
	}
	return "", fmt.Errorf("openai response had no content")
}
