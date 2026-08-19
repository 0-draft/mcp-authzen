# mcp-authzen

> **Moved.** This server now lives in [`kanywst/mcp-opa-authz`](https://github.com/kanywst/mcp-opa-authz), merged with `mcp-opa` into a single binary that exposes both `authzen_evaluate` and `evaluate_policy`. This repository's commit history is preserved there. This repo is archived and read-only.
>
> ```bash
> go install github.com/kanywst/mcp-opa-authz@latest
> ```

[![ci](https://github.com/0-draft/mcp-authzen/actions/workflows/ci.yml/badge.svg)](https://github.com/0-draft/mcp-authzen/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/0-draft/mcp-authzen.svg)](https://pkg.go.dev/github.com/0-draft/mcp-authzen)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](./LICENSE)

[MCP](https://modelcontextprotocol.io) server that fronts an [OpenID AuthZEN 1.0](https://openid.net/specs/authorization-api-1_0.html) PDP.

Sends a `subject + resource + action + context` bundle to a real Policy Decision Point (OPA-AuthZEN, Topaz, your own) and returns the decision. Use it when "can alice delete this?" should be answered by policy code, not by the model's guess.

Conforms to AuthZEN 1.0 §6 (single-evaluation request/response) and §5.5 (decision entity). Batch evaluation (§7) is not yet exposed as a tool — file an issue if you need it.

## Install

```bash
go install github.com/0-draft/mcp-authzen@latest
```

Pre-built signed binaries are on the [releases page](https://github.com/0-draft/mcp-authzen/releases).

## Quickstart

```bash
# Point at a real PDP (or your local opa-authzen-plugin on :8181)
export AUTHZEN_PDP_URL=http://localhost:8181/access/v1/evaluation

# Optional: PDP behind auth. Bare values are auto-prefixed with "Bearer ";
# values starting with "Bearer " or "Basic " pass through verbatim.
# export AUTHZEN_PDP_TOKEN=eyJhbGciOi...

# Run the smoke test (spins up an in-process fake PDP, exercises the
# full MCP handshake, asserts the decision is forwarded correctly)
make smoke
```

## Wire it to Claude Code

```bash
AUTHZEN_PDP_URL=http://localhost:8181/access/v1/evaluation \
  claude mcp add authzen -- mcp-authzen
```

Then in a session:

> Can `alice` (role=admin) `delete` `doc-42` (owner=bob)?

The model builds the AuthZEN bundle, calls `authzen_evaluate`, returns the PDP's decision.

## Wire it to Cursor / other clients

```jsonc
{
  "mcpServers": {
    "authzen": {
      "command": "mcp-authzen",
      "env": { "AUTHZEN_PDP_URL": "http://localhost:8181/access/v1/evaluation" }
    }
  }
}
```

## Tool: `authzen_evaluate`

| Param      | Required | Description                                                                |
| ---------- | -------- | -------------------------------------------------------------------------- |
| `subject`  | yes      | JSON object. AuthZEN §5.1, e.g. `{"type":"user","id":"alice"}`.            |
| `resource` | yes      | JSON object. AuthZEN §5.2, e.g. `{"type":"doc","id":"doc-1"}`.             |
| `action`   | yes      | JSON object. AuthZEN §5.3, e.g. `{"name":"read"}`.                         |
| `context`  | no       | JSON object with runtime context. AuthZEN §5.4.                            |
| `pdp_url`  | no       | Override `AUTHZEN_PDP_URL` for this call.                                  |

Returns AuthZEN's `{"decision": <bool>, "context": {...}}` as JSON.

## Test against opa-authzen-plugin

[`kanywst/opa-authzen-plugin`](https://github.com/kanywst/opa-authzen-plugin) is a reference AuthZEN PDP built on OPA.

```bash
# In one terminal — start the PDP on :8181
git clone https://github.com/kanywst/opa-authzen-plugin
cd opa-authzen-plugin && make run

# In another — wire mcp-authzen
export AUTHZEN_PDP_URL=http://localhost:8181/access/v1/evaluation
mcp-authzen
```

## Layout

Flat by design. A single-binary MCP server with one tool does not need
`cmd/`, `internal/`, or `pkg/`. When batch (`/access/v1/evaluations`) or
the search APIs (§8) land, they get sibling files, not subpackages.

```text
.
├── main.go         # server bootstrap + tool registration
├── main_test.go    # httptest-driven PDP round-trips
├── scripts/smoke.sh
├── .goreleaser.yml
└── .github/
```

## Verify a release

Releases ship a `cosign`-signed checksum file (Sigstore keyless via GitHub OIDC) and a CycloneDX SBOM per archive.

```bash
TAG=v0.1.0
gh release download "$TAG" -R 0-draft/mcp-authzen -p '*-checksums.txt*'

cosign verify-blob \
  --certificate "mcp-authzen-${TAG#v}-checksums.txt.pem" \
  --signature   "mcp-authzen-${TAG#v}-checksums.txt.sig" \
  --certificate-identity-regexp 'https://github.com/0-draft/mcp-authzen/.github/workflows/release.yml@refs/tags/' \
  --certificate-oidc-issuer 'https://token.actions.githubusercontent.com' \
  "mcp-authzen-${TAG#v}-checksums.txt"
```

## License

MIT.
