// Package llm provides minimal completion clients for the three supported
// backends — Anthropic (Claude), Google (Gemini), and OpenAI — over plain
// net/http. graphy uses an LLM for exactly one thing: interpreting a
// natural-language question into graph seed nodes. Everything else stays
// deterministic, and every query works offline via the keyword fallback.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// ErrNoBackend means no API key is configured; callers degrade to keyword search.
var ErrNoBackend = errors.New("no LLM API key configured (set ANTHROPIC_API_KEY, GEMINI_API_KEY, or OPENAI_API_KEY)")

// Backend is a single-turn completion client.
type Backend interface {
	Name() string
	Complete(ctx context.Context, system, user string) (string, error)
}

// FromEnv picks the first configured backend by key precedence:
// ANTHROPIC_API_KEY → GEMINI_API_KEY/GOOGLE_API_KEY → OPENAI_API_KEY.
// model overrides the backend's default model ("" keeps the default);
// the GRAPHY_LLM_MODEL environment variable is the flagless equivalent.
func FromEnv(model string) (Backend, error) {
	if model == "" {
		model = os.Getenv("GRAPHY_LLM_MODEL")
	}
	httpc := &http.Client{Timeout: 60 * time.Second}
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		return &Anthropic{Key: key, Model: model, HTTP: httpc}, nil
	}
	if key := firstEnv("GEMINI_API_KEY", "GOOGLE_API_KEY"); key != "" {
		return &Gemini{Key: key, Model: model, HTTP: httpc}, nil
	}
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		return &OpenAI{Key: key, Model: model, HTTP: httpc}, nil
	}
	return nil, ErrNoBackend
}

func firstEnv(names ...string) string {
	for _, n := range names {
		if v := os.Getenv(n); v != "" {
			return v
		}
	}
	return ""
}

// postJSON sends a JSON request and returns the raw response body,
// wrapping non-2xx statuses with a truncated body excerpt.
func postJSON(ctx context.Context, httpc *http.Client, url string, headers map[string]string, payload any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("content-type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := httpc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		excerpt := string(data)
		if len(excerpt) > 300 {
			excerpt = excerpt[:300] + "…"
		}
		return nil, fmt.Errorf("%s returned %d: %s", url, resp.StatusCode, excerpt)
	}
	return data, nil
}
