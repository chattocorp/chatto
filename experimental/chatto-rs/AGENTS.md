# Chatto Rust SDK

Read [README.md](README.md) for the current API and limits.

- Keep the crate independent of GPUI and all application packages.
- Keep public requests and protocol rules here. Hosts own presentation,
  credential storage, history retention, and runtime task ownership.
- Preserve unknown timeline events and unmodeled message metadata. Keep absent
  bodies distinct from present empty bodies. Do not apply bot-only filters to
  reads for human clients.
- Do not follow credential-bearing redirects or expose private data in errors
  and debug output. Keep response size and request duration bounded.
- Keep cursor scope, cancellation, and account-data resets explicit.
- Run `mise check` and `mise test` here and in `../chatto-native` after API changes.
