---
name: "chatto-jobs-implementation"
description: "Add, change, or remove a Chatto background job type, or change the policy, handler, or payload of one. Use before you enqueue new work on the JOBS queue in cli/internal/core, and when a job type shows as Unknown on the Jobs page."
---

# Chatto Background Job Implementation

Chatto runs durable background work on the shared job queue in `pkg/jobs`.
Chatto owns the job types, policies, handlers, payloads, and the Jobs page.
Read [ADR-123](../../../docs/adr/ADR-123-durable-background-jobs.md) and
[FDR-056](../../../docs/fdr/FDR-056-background-jobs.md) first. Read
[`pkg/jobs/AGENTS.md`](../../../pkg/jobs/AGENTS.md) only when you change the
shared module itself.

A job type touches the server, the public API, the frontend, every complete
translation catalog, and the architecture documentation. CI checks only the
server and frontend lists against
[`testdata/jobs/types.json`](../../../testdata/jobs/types.json). Use the
checklist below for the other parts.

## Rules

- A job type name is persisted in job facts and in JOBS subjects. Do not rename
  or reuse a name. Use `snake_case`.
- A job payload holds opaque references and policy only. Never put message
  content, email addresses, tokens, or endpoint credentials in a payload. Keep
  sensitive content in encrypted, expiring runtime state, as email does in
  [`email_jobs.go`](../../../cli/internal/core/email_jobs.go).
- Handlers must be idempotent. A crash after an external effect and before the
  job succeeds runs the attempt again.
- Handler errors are stored as safe failure categories. Never put personal data
  or secrets in the failure text.
- Keep domain completion rules in the handler, not in the queue.
- Use `jobs.UnlimitedAttempts` only for work that must finish, such as cleanup
  and key deletion. Set `ExpiresAt` when late work is useless.

## Checklist For A New Job Type

1. **Server type.** Add the type constant and add it to `jobTypes()` in
   [`jobs.go`](../../../cli/internal/core/jobs.go). This creates its JOBS
   consumer and its worker diagnostics row.
2. **Handler and pool.** Start a worker for the type in `runJobs`, or in the
   runtime that owns the transport, as `EmailJobs.Run` does.
3. **Enqueue.** Choose a stable job ID that deduplicates repeated handoffs.
   Set the attempts, retry delay, and expiry.
4. **Shared list.** Add the type to
   [`testdata/jobs/types.json`](../../../testdata/jobs/types.json).
5. **Frontend label.** Add a case to `knownJobTypeLabel` in
   `apps/frontend/src/routes/chat/[serverId]/manage/server/jobs/jobPresentation.ts`.
   Add `admin.jobs.<type>` to `apps/frontend/messages/en-GB/admin.json` and
   to every complete catalog. Follow the i18n rules in
   [`apps/frontend/AGENTS.md`](../../../apps/frontend/AGENTS.md).
6. **Public API comment.** Add the type to the comment of `Job.type` in
   [`jobs.proto`](../../../proto/chatto/admin/v1/jobs.proto). Run
   `mise codegen-proto`.
7. **Documentation.** Update these records:
   - the policy table in ADR-123,
   - the job types in FDR-056,
   - the job type list in `docs/architecture/subjects-and-events.md`,
   - the consumer table in `docs/architecture/nats-resources.md`,
   - `docs/architecture/durable-effects.md`, and
     `docs/architecture/runtime-state.md` when the job keeps runtime state.
   Use the
   [chatto-architecture-inventory](../chatto-architecture-inventory/SKILL.md)
   skill for the architecture files.
8. **Tests.** Test the handler, its retry and terminal outcomes, and its
   idempotency. Run the checks below.

## Change Or Remove A Job Type

- A policy change applies to new jobs. Jobs that are already queued keep the
  policy in their facts. Update the ADR-123 policy table.
- When you remove a type, retained facts of that type stay in JOBS until
  retention removes them. Keep its frontend label until no supported server
  can still hold those jobs.

## Verification

```sh
mise x -- go test ./cli/internal/core -run 'Job' -count=1
mise x -- pnpm --filter chatto-frontend exec vitest --run \
  src/routes/chat/[serverId]/manage/server/jobs/
mise license-check
```
