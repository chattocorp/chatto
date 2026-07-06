---
name: "chatto-security-review"
description: "Perform a comprehensive security review of the Chatto codebase. Launch multiple exploration agents in parallel to examine different security aspects, then have an adversarial reviewer verify and challenge the findings."
---

Perform a comprehensive, multi-agent security review of the Chatto codebase. Use the methodology below — it is designed to maximize coverage and minimize false positives.

## Phase 1: Parallel Specialized Review Agents

Launch **5 agents in parallel** using the Agent tool (subagent_type: general-purpose), each focused on a different attack surface. Each agent should:
- Read actual source code (not guess)
- Cite specific file paths and line numbers
- Rate severity as Critical/High/Medium/Low/Info
- Describe a concrete attack scenario for each finding
- Suggest a fix
- Also note positive security patterns (things done well)

### Agent 1: Authentication & Access Control
Focus: session management, token handling, password hashing, login/register/OAuth flows, authorization checks in ConnectRPC handlers and core operation models, IDOR vulnerabilities, privilege escalation, admin access control, server/room membership enforcement.

Key files: `cli/internal/http_server/auth.go`, `cli/internal/connectapi/`, `cli/internal/core/*_model.go`, `cli/internal/core/can.go`, `cli/internal/core/permissions.go`

### Agent 2: Input Validation & Injection
Focus: ConnectRPC/protobuf request validation, realtime websocket frame validation, XSS via stored data, path traversal, SSRF, command injection, deserialization issues, missing input validation on public operations.

Key files: `proto/chatto/api/v1/*.proto`, `cli/internal/connectapi/`, `cli/internal/http_server/realtime.go`, `cli/internal/core/validation.go`, `cli/cmd/backup.go`, `cli/internal/core/linkpreview/`

### Agent 3: Cryptography & Data Protection
Focus: encryption algorithm choices, nonce handling, key generation/storage/rotation, password hashing, secrets in code, transport security (TLS config), backup encryption, session token entropy, sensitive data in logs.

Key files: `cli/internal/encryption/`, `cli/internal/core/users.go`, `cli/cmd/keys.go`, `cli/cmd/backup.go`, `cli/cmd/init.go`, `cli/internal/http_server/server.go`

### Agent 4: Infrastructure & Configuration
Focus: CORS, rate limiting, realtime websocket security, cookie flags, HTTP security headers (CSP, HSTS), Docker/K8s security, file upload limits, DoS vectors (missing timeouts, unbounded operations), error message leakage.

Key files: `cli/internal/http_server/server.go`, `cli/internal/http_server/cors.go`, `cli/internal/http_server/frontend.go`, `cli/internal/http_server/health.go`, `docker/Dockerfile.goreleaser`, `examples/k8s/`

### Agent 5: Frontend Security
Focus: XSS via `{@html}`, auth token storage, CSRF protection, sensitive data in client bundle, open redirects, CSP compatibility, WebSocket message validation, file upload client-side validation, service worker security.

Key files: `apps/frontend/src/lib/markdown.ts`, `apps/frontend/src/lib/components/MessageContent.svelte`, `apps/frontend/src/lib/state/server/serverConnection.svelte.ts`, `apps/frontend/src/lib/state/server/eventBus.svelte.ts`, `apps/frontend/src/app.html`, `apps/frontend/src/service-worker.ts`

## Phase 2: Compile Findings

After all 5 agents complete, compile their findings into a single deduplicated report at `.context/security-review-findings.md`. Group by severity, note which findings were independently reported by multiple agents (higher confidence).

## Phase 3: Adversarial Review

Launch **1 final agent** (subagent_type: general-purpose) that is deeply skeptical and does NOT trust the other agents' work. This agent must:

1. **Read the compiled findings** from `.context/security-review-findings.md`
2. **Verify every finding** by reading the actual source code at cited file/line numbers
3. **Challenge severity ratings** — consider deployment context (self-hosted, typically behind reverse proxy)
4. **Check for false positives** — look for mitigations the auditors missed (middleware, framework protections, reverse proxy expectations)
5. **Look for things the auditors MISSED** — especially:
   - Race conditions in concurrent NATS operations
   - Subscription data leaks after membership changes
   - Logic errors in edge-case permission checks
   - ConnectRPC request-size or validation gaps
   - Realtime websocket replay or authorization bypasses
6. **Rate each finding** as: CONFIRMED, CONFIRMED-DOWNGRADE, CONFIRMED-UPGRADE, FALSE POSITIVE, or NEEDS-MORE-INVESTIGATION
7. **Add any new findings** discovered during verification

## Phase 4: Final Report

Save the verified, adversarially-reviewed report to `.context/security-review-final.md` with:
- Summary table of all findings with final severity ratings
- Recommended priority order for fixes (quick wins first)
- Positive security patterns confirmed
- Overall audit quality assessment

## Known Good Patterns (Context for Reviewers)

These are intentional design choices, not vulnerabilities:
- User profiles are public within an instance (by design)
- Instance assets (avatars, logos) are served without auth (by design)
- Public operation authorization lives in ConnectRPC handlers and shared core operation models; low-level helpers may still assume trusted callers by design.
- CORS defaults to wildcard without credentials for multi-instance support
- `CreateUser` mutation is unauthenticated (self-registration)
