# mcp-authzen

[![ci](https://github.com/0-draft/mcp-authzen/actions/workflows/ci.yml/badge.svg)](https://github.com/0-draft/mcp-authzen/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/0-draft/mcp-authzen.svg)](https://pkg.go.dev/github.com/0-draft/mcp-authzen)

A [Model Context Protocol](https://modelcontextprotocol.io) server that fronts an [OpenID AuthZEN 1.0](https://openid.net/specs/authorization-api-1_0.html) Policy Decision Point (PDP).

Lets an LLM agent ask **"can subject S perform action A on resource R?"** and have the answer come from a real authorization decision point — your OPA-AuthZEN, your Topaz, your in-house PDP — instead of from the model's training data.

## Tool

### `authzen_evaluate`

POST the bundle to the configured PDP's `/access/v1/evaluation` (or equivalent) endpoint, return the decision.

| Parameter   | Required | Description                                                                                     |
| ----------- | -------- | ----------------------------------------------------------------------------------------------- |
| `subject`   | yes      | JSON object: `{"type": "user", "id": "alice", "properties": {...}}`                             |
| `resource`  | yes      | JSON object: `{"type": "document", "id": "doc-1", "properties": {...}}`                         |
| `action`    | yes      | JSON object: `{"name": "read", "properties": {...}}`                                            |
| `context`   | no       | Free-form JSON object with runtime context (IP, time, MFA strength, etc).                       |
| `pdp_url`   | no       | Override the default PDP URL for this call.                                                     |

Returns AuthZEN's `{"decision": <bool>, "context": {...}}` as JSON.

## Configuration

Set the default PDP endpoint via env:

```bash
export AUTHZEN_PDP_URL=http://localhost:8181/access/v1/evaluation
```

Or pass `pdp_url` on every call. If neither is set, the tool returns an error.

## Install

```bash
go install github.com/0-draft/mcp-authzen@latest
```

Or grab a signed binary from the [releases page](https://github.com/0-draft/mcp-authzen/releases).

## Use with Claude Code

```bash
AUTHZEN_PDP_URL=http://localhost:8181/access/v1/evaluation \
  claude mcp add authzen -- mcp-authzen
```

Then in a session:

> Can `alice` (role=admin) `delete` `doc-42` (owner=bob)?

Claude builds the AuthZEN request, calls `authzen_evaluate`, returns the PDP's verdict — auditable, deterministic, and your policy never leaves your PDP.

## Use with Cursor / other MCP clients

```json
{
  "mcpServers": {
    "authzen": {
      "command": "mcp-authzen",
      "env": { "AUTHZEN_PDP_URL": "http://localhost:8181/access/v1/evaluation" }
    }
  }
}
```

## Test against a local PDP

Spin up [`opa-authzen-plugin`](https://github.com/kanywst/opa-authzen-plugin) (or any AuthZEN PDP) on `:8181`, then:

```bash
AUTHZEN_PDP_URL=http://localhost:8181/access/v1/evaluation mcp-authzen
```

## Verifying a release

Each release ships a `cosign`-signed checksum file (keyless, Sigstore via GitHub OIDC) and a CycloneDX SBOM. To verify before installing:

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
