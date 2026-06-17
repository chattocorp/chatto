# Security Policy

Chatto is alpha software. We are making the source available publicly so people
can self-host, inspect, and experiment with it, but the project does not yet
offer long-term stability or security support guarantees.

## Supported Versions

Security fixes are made on the active `main` release line. Self-hosters should
run the latest published release or Docker image unless they have pinned a
version intentionally and are prepared to evaluate fixes manually.

## Reporting a Vulnerability

Please do not open a public GitHub issue for a vulnerability.

Send reports to security@chattocorp.eu with:

- The affected Chatto version or commit SHA
- A clear description of the issue and impact
- Reproduction steps or a minimal proof of concept
- Any relevant deployment details, such as reverse proxy, TLS, or authentication
  configuration

We will acknowledge valid reports as quickly as practical and coordinate fixes
privately before publishing details.

## Scope

Reports about Chatto itself, the bundled frontend, Docker images, and deployment
examples are in scope. Reports about third-party services or user-managed
infrastructure are only in scope when Chatto's documented configuration creates
the issue.
