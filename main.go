// mcp-authzen is a Model Context Protocol (MCP) server that fronts an
// OpenID AuthZEN 1.0 compliant Policy Decision Point (PDP). It exposes
// `authzen_evaluate` as an MCP tool so an LLM agent can ask "can subject S
// perform action A on resource R?" and route the question to a real PDP.
//
// The PDP endpoint can be configured via the AUTHZEN_PDP_URL environment
// variable or overridden per-call via the optional `pdp_url` argument.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

var version = "dev"

// authzenRequest matches the OpenID AuthZEN 1.0 Evaluation API request body.
//
//	https://openid.net/specs/authorization-api-1_0.html
type authzenRequest struct {
	Subject  json.RawMessage `json:"subject"`
	Resource json.RawMessage `json:"resource"`
	Action   json.RawMessage `json:"action"`
	Context  json.RawMessage `json:"context,omitempty"`
}

type authzenResponse struct {
	Decision bool            `json:"decision"`
	Context  json.RawMessage `json:"context,omitempty"`
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--version", "-v", "version":
			fmt.Printf("mcp-authzen %s\n", version)
			return
		case "--help", "-h", "help":
			fmt.Println(`mcp-authzen — MCP server fronting an OpenID AuthZEN 1.0 PDP.

Usage:
  mcp-authzen           Run as an MCP stdio server.
  mcp-authzen --version Print version.

Configuration:
  AUTHZEN_PDP_URL       Default PDP evaluation endpoint, e.g.
                        http://localhost:8181/access/v1/evaluation

Tools exposed:
  authzen_evaluate      POST to the PDP's evaluation endpoint with a
                        subject/resource/action/context bundle.`)
			return
		}
	}

	s := server.NewMCPServer(
		"mcp-authzen",
		version,
		server.WithToolCapabilities(false),
	)

	s.AddTool(
		mcp.NewTool("authzen_evaluate",
			mcp.WithDescription(
				"Ask an OpenID AuthZEN 1.0 PDP whether a subject is allowed "+
					"to perform an action on a resource. Returns the PDP's "+
					"decision (true/false) and optional context."),
			mcp.WithString("subject",
				mcp.Required(),
				mcp.Description(`JSON object describing the principal. Per AuthZEN: `+
					`{"type": "user", "id": "alice", "properties": {...}}.`),
			),
			mcp.WithString("resource",
				mcp.Required(),
				mcp.Description(`JSON object describing the target. Per AuthZEN: `+
					`{"type": "document", "id": "doc-1", "properties": {...}}.`),
			),
			mcp.WithString("action",
				mcp.Required(),
				mcp.Description(`JSON object describing the action. Per AuthZEN: `+
					`{"name": "read", "properties": {...}}.`),
			),
			mcp.WithString("context",
				mcp.Description(`Optional JSON object with runtime context `+
					`(IP, time, MFA strength, etc).`),
			),
			mcp.WithString("pdp_url",
				mcp.Description(`Override the AUTHZEN_PDP_URL env. Must be a `+
					`full URL to the evaluation endpoint.`),
			),
		),
		evaluate,
	)

	if err := server.ServeStdio(s); err != nil {
		log.Fatalf("mcp-authzen: %v", err)
	}
}

var httpClient = &http.Client{Timeout: 10 * time.Second}

func evaluate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pdpURL := req.GetString("pdp_url", "")
	if pdpURL == "" {
		pdpURL = os.Getenv("AUTHZEN_PDP_URL")
	}
	if pdpURL == "" {
		return mcp.NewToolResultError(
			"no PDP URL: set AUTHZEN_PDP_URL or pass pdp_url"), nil
	}

	subject, err := parseJSONArg(req, "subject", true)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	resource, err := parseJSONArg(req, "resource", true)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	action, err := parseJSONArg(req, "action", true)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	contextRaw, err := parseJSONArg(req, "context", false)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	body, _ := json.Marshal(authzenRequest{
		Subject:  subject,
		Resource: resource,
		Action:   action,
		Context:  contextRaw,
	})

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, pdpURL, bytes.NewReader(body))
	if err != nil {
		return mcp.NewToolResultError("build request: " + err.Error()), nil
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	res, err := httpClient.Do(httpReq)
	if err != nil {
		return mcp.NewToolResultError("PDP request failed: " + err.Error()), nil
	}
	defer res.Body.Close()

	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return mcp.NewToolResultError("read PDP response: " + err.Error()), nil
	}

	if res.StatusCode >= 400 {
		return mcp.NewToolResultError(fmt.Sprintf(
			"PDP returned %d: %s", res.StatusCode, strings.TrimSpace(string(raw)))), nil
	}

	var decoded authzenResponse
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return mcp.NewToolResultError("PDP response is not valid AuthZEN JSON: " + err.Error()), nil
	}

	out, _ := json.MarshalIndent(decoded, "", "  ")
	return mcp.NewToolResultText(string(out)), nil
}

func parseJSONArg(req mcp.CallToolRequest, name string, required bool) (json.RawMessage, error) {
	s := req.GetString(name, "")
	if s == "" {
		if required {
			return nil, fmt.Errorf("missing required arg %q", name)
		}
		return nil, nil
	}
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return nil, fmt.Errorf("arg %q is not valid JSON: %w", name, err)
	}
	return json.RawMessage(s), nil
}
