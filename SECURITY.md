# Security Policy

## Supported Versions

Only the latest tagged release is actively supported. This is a single-developer
FOSS project; older releases do not receive backported fixes.

| Version | Supported |
| ------- | --------- |
| latest  | ✅        |
| older   | ❌        |

## Reporting

Please do **not** file a public GitHub issue for security matters.

Report privately via [GitHub Security Advisories](https://github.com/lmgarret/polar-flow-mcp/security/advisories/new),
or email the project maintainer directly (address on the GitHub profile of
[@lmgarret](https://github.com/lmgarret)).

We prefer coordinated disclosure. Please include a description of the issue,
steps to reproduce, and your assessment of impact. Best-effort acknowledgment
within 7 days.

## Scope

In scope:
- Bugs in the polar-flow-mcp server code itself
- Deployment defaults that could lead to credential exposure
- Documentation that could lead operators to insecure configurations

Out of scope:
- Issues in upstream dependencies — please report those upstream
- Social engineering
- Denial-of-service via load testing

## Threat Model

For the full threat model and security architecture, see
[docs/security.md](docs/security.md).
