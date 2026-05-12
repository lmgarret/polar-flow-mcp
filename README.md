# polar-flow-mcp

Multi-user MCP server wrapping the Polar AccessLink API for Claude.

[![CI](https://img.shields.io/github/actions/workflow/status/lmgarret/polar-flow-mcp/ci.yml?branch=main&label=CI)](https://github.com/lmgarret/polar-flow-mcp/actions/workflows/ci.yml)
[![License](https://img.shields.io/github/license/lmgarret/polar-flow-mcp)](LICENSE)
[![Latest Release](https://img.shields.io/github/v/release/lmgarret/polar-flow-mcp)](https://github.com/lmgarret/polar-flow-mcp/releases/latest)
[![Image Size](https://ghcr-badge.egpl.dev/lmgarret/polar-flow-mcp/size)](https://github.com/lmgarret/polar-flow-mcp/pkgs/container/polar-flow-mcp)
[![Go Report Card](https://goreportcard.com/badge/github.com/lmgarret/polar-flow-mcp)](https://goreportcard.com/report/github.com/lmgarret/polar-flow-mcp)

![Claude using create_training_target](docs/images/claude-tool-call.png)

polar-flow-mcp is a multi-user MCP server that lets you create, list, and delete Polar training targets directly from Claude conversations. It connects to the Polar AccessLink API via OAuth and supports multiple users through a reverse-proxy authentication layer. A bundled `polar-coach` skill gives Claude structured guidance for setting training intensity and heart-rate zones.

## Quickstart

```bash
git clone https://github.com/lmgarret/polar-flow-mcp.git
cp docker-compose.yml .env.yml  # copy and edit env vars
# Set ENCRYPTION_KEY, PROXY_SHARED_SECRET, POLAR_CLIENT_ID, POLAR_CLIENT_SECRET
docker compose up -d
# Visit https://lmgarret.github.io/polar-flow-mcp for full setup docs
```

## Documentation

Full documentation — Getting Started, Deployment, Usage, Reference, and Security — is available at:

**<https://lmgarret.github.io/polar-flow-mcp>**

## Features

- Create, list, and delete Polar training targets from Claude conversations
- Multi-user support via reverse-proxy auth (Authelia, Authentik, oauth2-proxy, Pomerium, Cloudflare Access)
- Encrypted OAuth token storage (AES-256-GCM, per-user)
- Fail-closed security: all requests require a verified proxy identity header
- Bundled `polar-coach` MCP skill for heart-rate zone and intensity guidance
- Single `FROM scratch` Docker image (~10 MB) with no runtime dependencies

## License

MIT — see [LICENSE](LICENSE).

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).
