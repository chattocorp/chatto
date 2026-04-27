# GraphQL Authorization Matrix Fuzzer

A targeted differential test for Chatto's GraphQL authorization boundary. The
tool runs every operation in its registry as a set of personas at known
privilege levels, then diffs the actual outcome against an expected matrix.
Any cell that doesn't match is a finding.

## Why this exists

Chatto's authorization model is enforced at the GraphQL gateway, not in core
(see [`.claude/rules/authorization.md`](../../.claude/rules/authorization.md)).
The contract is: every Query/Mutation/Subscription/field resolver is
responsible for calling the right `Can*` check before it touches `core`.
A single missed check on a single resolver = full bypass of that capability
for every user on the instance.

Manual review catches some of these. This catches them automatically and
keeps catching them as the schema grows.

The tool is also intended as a **regression guard**: when a security finding
gets fixed, add the matrix cell that codifies the new contract. If anyone
ever introduces the same bug again, the fuzzer fails.

## How it works

```
┌────────────┐   creates personas (anon, randomUser, …)
│  seed.ts   │── via /auth/test/create-user (build-tagged, dev/test only)
└────┬───────┘
     │
     ▼
┌────────────┐   builds the fixture world
│ pool/world │── publicSpace, publicRoom, otherSpace, seeded message
└────┬───────┘
     │
     ▼
┌────────────┐   for every (operation × persona) cell:
│  run.ts    │── call the operation as that persona
└────┬───────┘   classify the response (allow / deny / auth / notfound)
     │
     ▼
┌────────────┐   diff observed vs MATRIX
│  diff      │── exit non-zero on any mismatch
└────────────┘
```

## Personas

| ID | Description |
|---|---|
| `anon` | No session, no token. |
| `randomUser` | Authenticated; member of nothing. |
| `spaceMember` | Member of `seed.publicSpace`, default member role. |
| `roomMember` | `spaceMember` + joined `seed.publicRoom`. |
| `spaceAdmin` | Admin role on `seed.publicSpace`. |
| `otherSpaceOwner` | Owner of a *different* space — used to probe cross-tenant leakage. |
| `instanceAdmin` | Email matches `admin.emails` in instance config. **Currently degrades to a regular user** until the seed step is taught to wire this up; see "Known limitations" below. |

## Outcomes

For each `(operation, persona)` cell, the matrix declares one of:

- `allow` — call should succeed (200 + non-null result).
- `deny` — call should be rejected (typically `permission denied` or null).
- `auth` — call should be rejected as unauthenticated.
- `notfound` — call should return null *without* leaking existence (important
  for IDOR — a "deny" response leaks "this resource exists, you just can't
  see it").

Anything in the matrix that isn't explicitly listed for a persona defaults
to `deny`. **Silence is denial** — every `allow` is an explicit security
claim and forces the author to think.

The classifier in `run.ts` is deliberately loose because resolvers signal
denial inconsistently (some return null, some return an error message). The
matrix is the source of truth; the classifier just maps "did the user get
the data?" to one of the four buckets.

## Common findings the fuzzer surfaces

| Symptom | What it means |
|---|---|
| `expected deny, got allow` | **Authorization bypass.** A resolver missed its `Can*` check. |
| `expected notfound, got deny` | **Information leak via differential errors.** The resolver tells unauthorized callers that something exists. |
| `expected auth, got allow` | A resolver is missing `requireAuth`. |
| `expected allow, got deny` | Either the matrix is wrong, or a fix went too far and broke a legitimate use case. |

## Running

The fuzzer needs a Chatto instance built with the `test_endpoints` build tag
and reachable over HTTP. Local dev (`mise dev`) builds with this tag by
default, so an OrbStack URL works fine:

```sh
mise x -- node --experimental-strip-types tools/authz-fuzzer/run.ts \
  --endpoint=https://your-instance.orb.local
```

The fuzzer prints one line per mismatch and exits non-zero if any cell
diverges. Cells that match are silent.

The fuzzer creates fresh users on every run (`fuzz_random`, `fuzz_smember`,
…) and a fresh world. It does not clean up — runs accumulate state. If the
target instance starts to feel polluted, either tear it down (a fresh
`mise dev` rebuild gives you a clean DB) or extend the seed step to use
random suffixes per run.

### Why no `package.json`

The fuzzer has zero npm dependencies. It uses Node's built-in `fetch`, no
test runner, no transpiler, no Playwright. `node --experimental-strip-types`
(Node 22+) handles TypeScript natively. Keeping the surface area small makes
the tool easy to read, easy to run, and easy to wire into CI later without
worrying about lockfile drift.

If the matrix grows large enough to warrant a runner with parallelism /
reporters / etc., that's the moment to introduce a dependency — not before.

## Files

- `client.ts` — minimal GraphQL HTTP client with cookie jar; one instance
  per persona.
- `personas.ts` — persona registry. Adding a persona means adding an entry
  here and (usually) extending `seed.ts` to bring it into the world.
- `seed.ts` — bootstraps the fixture world. Creates users, builds the public
  space + room, posts a seed message used by edit/delete tests, creates the
  cross-tenant `otherSpace`.
- `operations.ts` — every operation under test. Each entry knows how to
  render its variables given the seed world. Returning `null` from `vars()`
  skips the op (when its prerequisites weren't seeded).
- `matrix.ts` — expected `(operation, persona) → outcome` table. **Read this
  file as a security claim**, not as a documentation of current behaviour.
- `run.ts` — orchestrator. Logs mismatches. Exits non-zero on any.

## Adding coverage

### A new operation

1. Append an entry to `OPERATIONS` in `operations.ts`. Pick `category`
   correctly (it changes how `null` data is classified).
2. Add a row to `MATRIX` in `matrix.ts`. Be explicit for every persona that
   should `allow` — anything you leave out defaults to `deny`.
3. Run the fuzzer. Either it passes (great) or it tells you a real bug or a
   wrong expectation.

### A new persona

1. Add an entry to `PERSONAS` in `personas.ts`. Order matters — least
   privileged first, since the seed step builds relationships incrementally.
2. Extend `buildClients` and `seed` in `seed.ts` to provision the new
   persona's role/membership.
3. Decide what every existing matrix row should expect for the new persona.
   This is the work — don't skip it. Every undocumented cell silently
   defaults to `deny`, which is sometimes wrong.

### A new outcome

If you find yourself wanting a new outcome label (e.g. `rate-limited`),
think hard first — the four-outcome model is intentionally minimal. Most
"new" outcomes are special cases of `deny`. If you genuinely need it,
extend the `Outcome` type in `matrix.ts` and the classifier in `run.ts`.

## Known limitations

- **`instanceAdmin` requires config alignment on the target instance.** The
  seed verifies each persona's email via `/auth/test/verify-email`, but the
  GraphQL `Query.admin` resolver only treats a user as admin if a verified
  email matches an entry in `admin.emails` in `chatto.toml`. Add the
  default persona email to your test instance's config:
  ```toml
  [admin]
  emails = ["fuzz_iadmin@example.test"]
  ```
  Until that's done on the target instance, `instanceAdmin → allow` cells
  fail-closed. Treat them as low-confidence rather than authoritative.
- **Subscriptions aren't tested.** Equally important attack surface; needs a
  WS client and time-bounded "did we receive an event?" assertions. The
  classifier and persona model already work for subscriptions in principle
  — what's missing is the transport.
- **The `Upload` scalar isn't actually uploaded.** Operations like
  `uploadSpaceLogo` test only the resolver-entry authz path, which is what
  we want for matrix coverage. End-to-end upload behaviour belongs in a
  separate harness.
- **No cleanup.** Each run leaves users and a world behind. Runs are
  idempotent in the sense that they don't break each other, but the target
  instance accumulates state. For long-lived dev instances, restart the
  stack occasionally.

## CI integration (future)

The intended deployment shape is a CI job that:

1. Spins up a Chatto instance built with `-tags test_endpoints`.
2. Runs the fuzzer against it with a fresh DB.
3. Fails the build on any mismatch.

Wiring this in needs:

- A reusable "ephemeral Chatto" GitHub Actions step (the e2e job already has
  one; reuse it).
- `instanceAdmin` actually working (see Known limitations) so the admin
  cells aren't silently low-confidence.
- A few iterations of running it against `main` to surface and either fix or
  matrix-document any pre-existing divergences.

Until that lands, run the fuzzer locally before merging anything that touches
`*.resolvers.go`, `core/can.go`, or `core/permissions.go`.

## Relationship to other testing

| Tool | Catches | Doesn't catch |
|---|---|---|
| **Go unit tests** in `cli/internal/graph/*_test.go` | Specific resolvers' authz logic at the function level. | Cross-resolver consistency, missing checks on resolvers nobody wrote tests for. |
| **Playwright e2e** in `frontend/e2e/` | UI-level happy paths and a few negative cases. | Comprehensive authz matrix; tests are slow and per-feature, not per-(resolver, persona). |
| **This fuzzer** | Every operation × every persona, declared as security claims. | Behaviour beyond the matrix outcomes (data correctness, performance, UX). |

The three layers are complementary. The fuzzer's job is to be the *broad*
guard against whole classes of bypass. Specific business rules and edge
cases stay where they are.
