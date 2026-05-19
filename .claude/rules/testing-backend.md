---
paths: ["cli/**"]
---

# Backend Testing

Patterns and gotchas for Go tests in `cli/`. See `testing-frontend.md` for Vitest / Playwright guidance.

## Run Go Tests via `mise test-cli`

Always use `mise test-cli`, **not** `go test` directly. The `http_server` package requires `-tags test_endpoints` to enable mock email endpoints used by tests. Running without this tag causes test failures with 404 errors on test endpoints like `/auth/test/last-email`.

## Use Table-Driven Tests Where Possible

Idiomatic Go: a `struct { name string; in X; want Y }` slice with a single `t.Run(tc.name, ...)` loop. Easier to read, easier to extend.

## Mocks and Fakes for Unit Tests

Use mocks and fakes to isolate components in unit tests. Integration tests can hit real NATS via the embedded server.

## DM Rooms Need Explicit Test Coverage

The DM space has different initialization (system-created at startup, not user-triggered) and different membership patterns (auto-joined on creation). **Changes to room, message, or space logic should always include DM-specific tests** — unit tests for regular rooms passing doesn't guarantee DM rooms work.

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
