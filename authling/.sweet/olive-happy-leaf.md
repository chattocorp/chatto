---
type: breaking
---

Add five-minute JWT access tokens for independently validated API access, exact resource allowlists, and rotating refresh tokens with explicit offline consent. Configured clients use `resources`; CIMD access requires `allow_cimd` on each registered resource. Refresh families default to 30 days total and seven days idle, and are invalidated by reuse, grant revocation, password reset, or password change. Email change preserves families. CIMD metadata must explicitly declare the `refresh_token` grant; configured clients can disable refresh with `allow_refresh_tokens = false`.

Upgrade all Authling replicas together. All users must consent once after upgrading to consent disclosure version 3; older binaries cannot replay new grants, so rollback is unsupported after the first version-3 grant. New exchanges issue JWT access tokens; existing opaque tokens remain usable at UserInfo until expiry. Clients must store each replacement token. The previous token can repeat a refresh once within a grace period after a rotation (`refresh_token_reuse_grace`, default `10s`, `0s` disables); any other reuse revokes the family. A narrowed refresh `scope` affects only that response; the replacement refresh token keeps the original scope.
