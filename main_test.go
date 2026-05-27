package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func callEval(t *testing.T, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	req := mcp.CallToolRequest{}
	req.Params.Name = "authzen_evaluate"
	req.Params.Arguments = args
	res, err := evaluate(context.Background(), req)
	if err != nil {
		t.Fatalf("evaluate returned non-nil err: %v", err)
	}
	return res
}

func resultText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if len(res.Content) == 0 {
		t.Fatal("empty content")
	}
	tc, ok := mcp.AsTextContent(res.Content[0])
	if !ok {
		t.Fatalf("first content is not text: %T", res.Content[0])
	}
	return tc.Text
}

// fakePDP returns the configured decision and echoes the request for inspection.
func fakePDP(t *testing.T, decision bool, capture *authzenRequest) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if capture != nil {
			_ = json.Unmarshal(body, capture)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(authzenResponse{Decision: decision})
	}))
}

func TestEvaluate_Allow(t *testing.T) {
	got := &authzenRequest{}
	pdp := fakePDP(t, true, got)
	defer pdp.Close()

	res := callEval(t, map[string]any{
		"pdp_url":  pdp.URL,
		"subject":  `{"type":"user","id":"alice"}`,
		"resource": `{"type":"doc","id":"doc-1"}`,
		"action":   `{"name":"read"}`,
	})
	if res.IsError {
		t.Fatalf("expected success, got error: %s", resultText(t, res))
	}
	if !strings.Contains(resultText(t, res), `"decision": true`) {
		t.Fatalf("expected decision=true in output: %s", resultText(t, res))
	}
	if string(got.Subject) != `{"type":"user","id":"alice"}` {
		t.Fatalf("subject not forwarded: %s", string(got.Subject))
	}
}

func TestEvaluate_Deny(t *testing.T) {
	pdp := fakePDP(t, false, nil)
	defer pdp.Close()

	res := callEval(t, map[string]any{
		"pdp_url":  pdp.URL,
		"subject":  `{"type":"user","id":"bob"}`,
		"resource": `{"type":"doc","id":"doc-1"}`,
		"action":   `{"name":"delete"}`,
	})
	if !strings.Contains(resultText(t, res), `"decision": false`) {
		t.Fatalf("expected decision=false: %s", resultText(t, res))
	}
}

func TestEvaluate_MissingPDP(t *testing.T) {
	t.Setenv("AUTHZEN_PDP_URL", "")
	res := callEval(t, map[string]any{
		"subject":  `{"id":"a"}`,
		"resource": `{"id":"r"}`,
		"action":   `{"name":"x"}`,
	})
	if !res.IsError {
		t.Fatal("expected error when PDP URL unset")
	}
}

func TestEvaluate_BadJSON(t *testing.T) {
	res := callEval(t, map[string]any{
		"pdp_url":  "http://example.invalid",
		"subject":  `{not json}`,
		"resource": `{"id":"r"}`,
		"action":   `{"name":"x"}`,
	})
	if !res.IsError {
		t.Fatal("expected error for malformed subject JSON")
	}
}

func TestEvaluate_BearerToken(t *testing.T) {
	t.Setenv("AUTHZEN_PDP_TOKEN", "s3cret")

	var gotAuth string
	pdp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(authzenResponse{Decision: true})
	}))
	defer pdp.Close()

	_ = callEval(t, map[string]any{
		"pdp_url":  pdp.URL,
		"subject":  `{"id":"a"}`,
		"resource": `{"id":"r"}`,
		"action":   `{"name":"x"}`,
	})
	if gotAuth != "Bearer s3cret" {
		t.Fatalf("Authorization header = %q, want %q", gotAuth, "Bearer s3cret")
	}
}

func TestEvaluate_BearerToken_PreservesPrefix(t *testing.T) {
	t.Setenv("AUTHZEN_PDP_TOKEN", "Bearer already-prefixed")

	var gotAuth string
	pdp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(authzenResponse{Decision: true})
	}))
	defer pdp.Close()

	_ = callEval(t, map[string]any{
		"pdp_url":  pdp.URL,
		"subject":  `{"id":"a"}`,
		"resource": `{"id":"r"}`,
		"action":   `{"name":"x"}`,
	})
	if gotAuth != "Bearer already-prefixed" {
		t.Fatalf("Authorization header = %q; expected raw passthrough", gotAuth)
	}
}

func TestEvaluate_PDP5xx(t *testing.T) {
	pdp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer pdp.Close()

	res := callEval(t, map[string]any{
		"pdp_url":  pdp.URL,
		"subject":  `{"id":"a"}`,
		"resource": `{"id":"r"}`,
		"action":   `{"name":"x"}`,
	})
	if !res.IsError {
		t.Fatal("expected error from 5xx response")
	}
	if !strings.Contains(resultText(t, res), "500") {
		t.Fatalf("expected status 500 in error: %s", resultText(t, res))
	}
}
