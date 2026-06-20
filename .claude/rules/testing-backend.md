---
paths: ["cli/**"]
---

# Backend Testing

Patterns and gotchas for Go tests in `cli/`. See `testing-frontend.md` for Vitest / Playwright guidance.

## Run Go Tests via `mise test-cli`

Always run Go tests through `mise` so the repo toolchain is active. For full-suite checkpoints, use `mise test-cli`, **not** plain `go test ./...`. The `http_server` package has endpoint-specific tests that require `-tags test_endpoints` to enable mock email/webhook routes such as `/auth/test/last-email`; tests for those routes are build-tagged and only run when the tag is present.

## Iterate with targeted runs; reserve `mise test-cli` for checkpoints

The full suite is still broad enough to be a checkpoint, not an edit-refresh loop. While iterating on a change, **don't run the whole suite as your feedback loop** — it wastes wall-clock time and buries real signal in noise. Instead:

- Drill into the specific package and test you care about:
  ```
  mise x -- go test ./internal/<pkg> -run TestX -timeout 30s
  ```
  Multiple tests via regex: `-run "TestX|TestY"`.
- **If the package is `internal/http_server` and you need test-only endpoint coverage, add `-tags test_endpoints`:**
  ```
  mise x -- go test -tags test_endpoints ./internal/http_server -run TestX -timeout 30s
  ```
  The same applies to any cross-package targeted run that must exercise `/auth/test/*` or `/webhooks/test/*`. Without the tag, those routes are intentionally compiled out; tagged tests for them will not run.
- **Always set `-timeout`.** A hung test will otherwise peg the session for the Go default of 10 minutes. 30s for unit-ish tests, 60s for ones that exercise live-event delivery.
- On a suspected flake, retry with `-count=3 -timeout 60s` to confirm flake-vs-bug before reading the failure as definitive.
- Reserve `mise test-cli` for "I think I'm done with this chunk" checkpoints, not feedback-loop iteration.
- Inside `cli/`, you can call `mise x -- go test …` directly; outside, prefix with `cd cli && …` or use absolute paths.

The same goes for cross-package work: prefer `mise x -- go test ./internal/core/ ./internal/graph/ -run TestX -timeout 60s` over the whole suite when you're chasing a specific change's blast radius. If that targeted set needs test-only `http_server` endpoints, use `mise x -- go test -tags test_endpoints ...`.

## Use Table-Driven Tests Where Possible

Idiomatic Go: a `struct { name string; in X; want Y }` slice with a single `t.Run(tc.name, ...)` loop. Easier to read, easier to extend.

## Mocks and Fakes for Unit Tests

Use mocks and fakes to isolate components in unit tests. Integration tests can hit real NATS via the embedded server.

## DM Rooms Need Explicit Test Coverage

DM rooms have different creation and membership patterns (participants are auto-joined on creation). **Changes to room or message logic should always include DM-specific tests** — unit tests for channel rooms passing doesn't guarantee DM rooms work.

## Permission Tests Need Both Positive and Negative Cases

When testing authorization/permissions, always test both directions:

- **Positive**: user WITH the permission CAN access/perform the action
- **Negative**: user WITHOUT the permission is DENIED access

Missing negative tests means you don't know if permission checks are actually enforced. This applies to resolver tests as much as e2e tests.

## Email Testing

| Tool           | Purpose                                           | Location                                   |
| -------------- | ------------------------------------------------- | ------------------------------------------ |
| `MockSender`   | Capture emails in memory for business logic tests | `internal/email/mock.go`                   |
| `go-smtp-mock` | Test actual SMTP protocol with go-mail library    | `internal/email/email_integration_test.go` |

**`go-smtp-mock` quirks**:
- Set `MultipleMessageReceiving: true`.
- Use `server.WaitForMessages(count, timeout)` instead of `server.Messages()` to avoid races.

## Run E2E Tests Before Committing Refactors

Unit tests passing doesn't guarantee the system works end-to-end. For refactoring work that touches data flow (subjects, streams, queries), run e2e tests before committing to catch integration issues. See `testing-frontend.md` for how to run them locally.
