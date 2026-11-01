# Chatto Go SDK

This module is a framework-neutral integration library. Read [README.md](README.md)
for its APIs and lifecycle rules.

- Keep three boundaries: stateless API requests, optional connection and stores,
  and explicit convenience helpers such as bot observers and message projections.
- Preserve server protocol data. Room groups and attention counts are protocol
  resources; sidebar visibility, badges, selection, and formatting belong to hosts.
- Do not infer authorization from navigation metadata. The API checks access.
  Preserve cancellation, privacy resets, and stale-response protection.
- Keep image dimensions, display filters, cache retention, credential storage,
  logging, and model-input selection in the consuming application.
- Let callers select pagination and resource limits. Keep secure transport
  boundaries: no credential-bearing redirects, cross-origin asset downloads,
  or private response data in errors.
- Keep bot-observer filtering explicit. Changes for human clients must not
  silently change Gobotto's input or durable inbox format.
- Public state reads must return detached data. Independent subscribers must
  not consume each other's signals. Document ownership and cancellation.
- Run `mise check` here and in `../chatty`, and test `../gobotto` after API
  changes. Use synthetic data. Keep builds independent of CGO.
