package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// capture spins an httptest server that records the request and returns body.
func capture(t *testing.T, respBody string, got *http.Request, gotBody *map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*got = *r
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("request body: %v", err)
		}
		*gotBody = body
		w.Header().Set("content-type", "application/json")
		w.Write([]byte(respBody))
	}))
}

func TestAnthropicRequestShape(t *testing.T) {
	var req http.Request
	var body map[string]any
	srv := capture(t, `{"content":[{"type":"text","text":"hello"}]}`, &req, &body)
	defer srv.Close()

	b := &Anthropic{Key: "sk-test", Base: srv.URL, HTTP: srv.Client()}
	out, err := b.Complete(context.Background(), "sys", "user prompt")
	if err != nil || out != "hello" {
		t.Fatalf("Complete = %q, %v", out, err)
	}
	if req.URL.Path != "/v1/messages" {
		t.Errorf("path = %s", req.URL.Path)
	}
	if req.Header.Get("x-api-key") != "sk-test" || req.Header.Get("anthropic-version") != "2023-06-01" {
		t.Errorf("headers = %v", req.Header)
	}
	if body["model"] != DefaultAnthropicModel || body["system"] != "sys" {
		t.Errorf("body = %v", body)
	}
	if body["max_tokens"] == nil {
		t.Errorf("max_tokens missing: %v", body)
	}
}

func TestGeminiRequestShape(t *testing.T) {
	var req http.Request
	var body map[string]any
	srv := capture(t, `{"candidates":[{"content":{"parts":[{"text":"hi"}]}}]}`, &req, &body)
	defer srv.Close()

	b := &Gemini{Key: "g-test", Model: "gemini-x", Base: srv.URL, HTTP: srv.Client()}
	out, err := b.Complete(context.Background(), "sys", "u")
	if err != nil || out != "hi" {
		t.Fatalf("Complete = %q, %v", out, err)
	}
	if req.URL.Path != "/v1beta/models/gemini-x:generateContent" {
		t.Errorf("path = %s", req.URL.Path)
	}
	if req.Header.Get("x-goog-api-key") != "g-test" {
		t.Errorf("headers = %v", req.Header)
	}
	if body["system_instruction"] == nil || body["contents"] == nil {
		t.Errorf("body = %v", body)
	}
}

func TestOpenAIRequestShape(t *testing.T) {
	var req http.Request
	var body map[string]any
	srv := capture(t, `{"choices":[{"message":{"content":"yo"}}]}`, &req, &body)
	defer srv.Close()

	b := &OpenAI{Key: "oa-test", Base: srv.URL, HTTP: srv.Client()}
	out, err := b.Complete(context.Background(), "sys", "u")
	if err != nil || out != "yo" {
		t.Fatalf("Complete = %q, %v", out, err)
	}
	if req.URL.Path != "/v1/chat/completions" {
		t.Errorf("path = %s", req.URL.Path)
	}
	if req.Header.Get("Authorization") != "Bearer oa-test" {
		t.Errorf("headers = %v", req.Header)
	}
	if body["model"] != DefaultOpenAIModel {
		t.Errorf("body = %v", body)
	}
}

func TestNon2xxIsWrapped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
		w.Write([]byte(`{"error":{"type":"rate_limit_error"}}`))
	}))
	defer srv.Close()

	b := &Anthropic{Key: "k", Base: srv.URL, HTTP: srv.Client()}
	_, err := b.Complete(context.Background(), "s", "u")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "429") || !strings.Contains(err.Error(), "rate_limit_error") {
		t.Errorf("error = %v", err)
	}
}

func TestFromEnvPrecedence(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("GOOGLE_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")

	if _, err := FromEnv(""); err != ErrNoBackend {
		t.Errorf("no keys: err = %v, want ErrNoBackend", err)
	}

	t.Setenv("OPENAI_API_KEY", "oa")
	if b, _ := FromEnv(""); b == nil || b.Name() != "openai" {
		t.Errorf("openai precedence failed: %v", b)
	}
	t.Setenv("GEMINI_API_KEY", "gm")
	if b, _ := FromEnv(""); b == nil || b.Name() != "gemini" {
		t.Errorf("gemini should outrank openai: %v", b)
	}
	t.Setenv("ANTHROPIC_API_KEY", "an")
	if b, _ := FromEnv(""); b == nil || b.Name() != "anthropic" {
		t.Errorf("anthropic should outrank gemini: %v", b)
	}
}
