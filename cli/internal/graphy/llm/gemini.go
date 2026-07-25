package llm

import (
	"context"
	"encoding/json"
	"fmt"
)

// Gemini calls the Google Generative Language API.
type Gemini struct {
	Key   string
	Model string // "" → DefaultGeminiModel
	Base  string // "" → https://generativelanguage.googleapis.com
	HTTP  httpDoer
}

// DefaultGeminiModel is the default Gemini model.
const DefaultGeminiModel = "gemini-2.5-flash"

func (g *Gemini) Name() string { return "gemini" }

func (g *Gemini) Complete(ctx context.Context, system, user string) (string, error) {
	base := g.Base
	if base == "" {
		base = "https://generativelanguage.googleapis.com"
	}
	model := g.Model
	if model == "" {
		model = DefaultGeminiModel
	}
	payload := map[string]any{
		"system_instruction": map[string]any{
			"parts": []map[string]string{{"text": system}},
		},
		"contents": []map[string]any{
			{"role": "user", "parts": []map[string]string{{"text": user}}},
		},
	}
	headers := map[string]string{"x-goog-api-key": g.Key}
	url := fmt.Sprintf("%s/v1beta/models/%s:generateContent", base, model)
	data, err := postJSON(ctx, asClient(g.HTTP), url, headers, payload)
	if err != nil {
		return "", err
	}
	var out struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", fmt.Errorf("gemini response: %w", err)
	}
	for _, c := range out.Candidates {
		for _, p := range c.Content.Parts {
			if p.Text != "" {
				return p.Text, nil
			}
		}
	}
	return "", fmt.Errorf("gemini response had no text content")
}
