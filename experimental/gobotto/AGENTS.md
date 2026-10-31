# Gobotto development instructions

Root repository instructions still apply. Read [README.md](README.md) for current
configuration, behavior, limits, and data flows.

## Documentation boundary

**Do not create or maintain FDRs for Gobotto**, either in Chatto's `docs/fdr/` or
in a local FDR directory. This is an explicit exception to the repository's general
FDR rule. Gobotto is an experiment, not a Chatto product feature.

Keep operator-facing behavior and setup in this directory's README. Keep durable
development rules here. If a design needs more detail, use an ordinary Markdown
document under this directory and link it from the README. Do not duplicate the
README as a separate feature record. Do not add Gobotto's runtime to Chatto's
server architecture inventory.

## Module boundaries

- Keep Gobotto in its own Go module. It is separate from ChattoBot and does not
  replace the supported TypeScript client.
- Put reusable Chatto connection and protocol behavior in the separate
  [chatto-go module](../chatto-go/). Gobotto uses its public API. Keep model,
  memory, and TUI behavior out of that prospective SDK.
- Keep the build CGO-free. Do not add dependencies to the Chatto server for this
  experiment.

## Runtime invariants

- Keep one retained model conversation per Chatto thread. A verified mention or
  DM can open a session; later messages in that thread use the same session.
  Do not infer mentions from plain text. Ignore bot self-messages and echoes.
- Debounce by resetting the quiet-period timer on each incoming message. Preserve
  append-only model history during ordinary turns. Refresh context when access,
  history changes, or restart require it, not before every message.
- Keep output bound to the triggering thread. The model must not select another
  destination. Preserve completed tool receipts even when later work fails.
- Do not automatically replay started turns or uncertain writes. A durable inbox
  can restore pending input; it must not turn an interrupted action into a retry.
  Model-tool work and Chatto delivery are not one transaction.
- For edits, compare model-visible message content before cancelling. Preview-only
  changes must not cancel unchanged work. Failed verification, retractions, and
  access changes remain conservative. Delivery waits for an active edit check.
- Preserve bounded work and a final text-only progress report at the work limit.
  Do not execute tools after that limit, even if the provider ignores tool choice.
- Use host-controlled typing indicators and first-session acknowledgements. Their
  failures must not fail a conversation turn or cause duplicate reactions.

## Ownership, means, and external tools

- Ownership belongs to configured bot user IDs, never to a thread, room, name,
  or claim in a message. Use each current message's server-supplied author.
  Historical labels and an owner's presence do not authorize other participants.
- Enforce privileged operations in host code. Means brain creation and archival
  require an entirely owner-authored batch. Prompt-defined participation rules
  guide the model; they are not data-isolation guarantees.
- Source collection titles, descriptions, schemas, and guidance from `MEANS.md`
  through `means info`. Do not duplicate them in `gobotto.toml`.
- Run means as a bounded JSON subprocess without a shell. Restrict collection
  paths to configured bindings and validated managed directories. Preserve the
  guidance-read prerequisite, current-turn hash checks, write receipts, and the
  block on further writes after an uncertain mutation.
- A corrected creation attempt is safe only when the tool explicitly establishes
  that nothing was mutated. Never overwrite an existing brain to recover a failure.
- Treat external results and ordinary stored documents as data, not authorization.
  Use only configured MCP capabilities. Do not replay failed external calls.

## Inspection and privacy

- Keep operational logs free of private conversation content. Full transcripts,
  tool arguments, and responses belong in private session storage and inspection.
- Treat `.gobotto/`, `brains/`, and `.env` as private runtime data. Do not commit
  them or silently replace them during development. Evaluate saved runs read-only
  unless the user asks for a repair.
- Preserve stable human session IDs, reported usage, and explicit unknown/partial
  cost values. Do not infer successful delivery from a model's statement.
- A missing latest inspector reply can be recovered only from a terminal assistant
  reply followed by a successful host delivery receipt. Do not borrow that reply
  for an older, interrupted, or uncertain turn.
- Keep UI snapshots detached from worker state. Preserve mouse hit areas when
  changing pane geometry. Normal quit drains the active turn; Shift-Q cancels it.
- Document external data flows in the README. Model providers receive supplied
  context; MCP servers receive tool arguments and the bot's outbound IP. Means
  can contact an embedding provider when the operator enables that feature.

## Verification

Run Go commands through `mise` from this directory; its configuration sets
`GOWORK=off` and loads `.env`. Do not print environment credentials.

- Use `mise x -- go test ./...` and `mise x -- go vet ./...` for Gobotto changes.
- Use targeted race tests for worker, intake, persistence, or UI concurrency changes.
- Check `CGO_ENABLED=0 mise x -- go build` when changing runtime dependencies.
- If changing the SDK, run its tests separately in `../chatto-go/`.
- Keep evaluation fixtures synthetic. Unit tests can verify host permissions and
  tool contracts; they do not prove that a live model will follow prompt guidance.

## Open design questions

These are unresolved, not implemented behavior:

- Whether replay-cursor expiry should trigger a bounded scan for missed messages.
- How to summarize long conversations without losing authority or write receipts.
- When the experimental Go client should become a supported SDK.
- Which additional tools and preconfigured subagents the harness should support.
