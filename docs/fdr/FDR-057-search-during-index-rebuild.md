# FDR-057: Search During Index Rebuild

**Status:** Planned
**Last reviewed:** 2026-11-17

## Overview

The bundled search provider must sometimes build its index again from `EVT`.
This occurs after a known index-format or language change, and after an
operator starts an explicit recovery ([#3663](https://github.com/chattocorp/chatto/issues/3663)). Today, Search is unavailable
until the full replay completes, and on a large server this can take hours.
This feature lets the provider continue to answer queries from its current
index while it builds a replacement index in a separate directory. The
provider changes to the replacement index in one atomic step, and only after
the replacement index is current and a real query on it succeeds. Results
tell the client when they can be stale. Search must never show an empty
result from an incomplete index as "no matches".

This record is a design. It does not describe shipped behavior. Each claim
about current code has a marker: **[verified]** means that the author read
the code at `aaafb82e10`; **[inferred]** means that the claim comes from
library documentation or reasoning and the implementation must confirm it
with a test.

## Behavior

- During a rebuild, members can search. The provider answers from the
  **serving index**, which is the last index that was complete and valid.
- The provider builds the **replacement index** in a separate directory below
  the configured search directory. The replacement index does not answer
  queries until the provider switches to it.
- The serving index continues to apply new events while the replacement index
  builds. New messages, edits, retractions, room deletions, and
  crypto-shredding continue to change the serving index.
- When the serving index is behind the event log, each result page tells the
  client that results can be stale. The client shows a short notice, for
  example "Search is updating. Recent messages can be missing." The notice
  does not show exact counts or rates (FDR-033, decision 4).
- After an ordinary restart, the provider answers from its checkpointed index
  while it replays the tail of the event log. Results are marked stale until
  the tail replay completes.
- The provider switches to the replacement index only when all these
  conditions are true:
  - The replacement index completed its replay.
  - The replacement index is at least as current as the serving index.
  - A real query through the replacement index's read path succeeds in a
    bounded time and returns at least one hit when the replay indexed at least
    one visible message.
- The switch is atomic. A query uses the old index or the new index, never a
  mixture. Queries that are in progress during the switch complete on the old
  index.
- If the replacement build or its validation fails, the serving index
  continues to answer queries. The provider reports the failure to operators.
  It does not delete the serving index.
- If no usable serving index exists, for example on first start or with an
  unreadable index, the behavior does not change: the provider reports
  indexing and does not answer queries. The first implementation does not
  serve partial results from a fresh index (decision 5).
- Pagination continues to read the live index (FDR-033). A switch between two
  pages can move, repeat, or remove hits.
- Authorization, deletion filtering, and body-revision checks do not change.
  Chatto continues to treat every provider hit as a candidate only.

## Design Decisions

### 1. Removing the startup gate is not the fix

**Decision:** Keep a serving gate. Change what it requires: the provider joins
the query queue when it has a usable serving index, not only when startup
replay is complete.
**Why:** Today the provider registers its query endpoint only after replay
completes [verified: `unit.go`, `waitForSearchProjectionStartup`], and
`Provider.Query` also rejects queries until `StartupComplete` [verified:
`provider.go`]. If we only remove the gate, three problems occur:

- After a known format change, `rebuildIndex` closes and deletes the old index
  before replay starts [verified: `rebuild.go`]. The provider would answer
  from an empty, growing index. An empty page would look like "no matches".
- Each index commit holds the exclusive projection lock for the full batch,
  including decryption and the Bleve commit [verified: `projection.go`,
  `applyBatch`]. Queries take the shared lock [verified: `query.go`]. During
  replay, commits run without pause, so queries wait behind them. A blocked
  commit blocks all queries.
- The public state `INDEXING` means "cannot answer queries yet" [verified:
  `search.proto` and `message_search.proto`]. Answers during that state would
  contradict the contract.

A **usable serving index** has a valid checkpoint for the current EVT
incarnation, uses the current or a known previous index contract, and passes
the [#3662](https://github.com/chattocorp/chatto/issues/3662) query probe. An empty index without a checkpoint is never usable.

**Tradeoff:** The design needs a second index directory, a second projector,
and a switch step. This is more work than a one-line change.

### 2. Build side by side, then switch atomically

**Decision:** Put each index in its own **index generation** directory below
the configured `search_provider.directory`. A small pointer file names the
active generation. To switch, the provider writes the new pointer to a
temporary file, syncs it, renames it over the old pointer, and syncs the
directory. Then it publishes the new index to queries in process with an
atomic reference. It stops the old projector, waits until the old index has no
queries in progress, and closes it.
**Why:** The configured directory is usually a dedicated volume mount. A
sibling directory outside the mount can be on a different filesystem, so a
rename there is not atomic. A pointer file in the same directory gives one
atomic, durable switch. The existing directory lock already covers the full
configured directory, so it covers all generations [verified: `rebuild.go`,
`lockDirectory`].

Each generation keeps its checkpoint inside its own Bleve index, as today
[verified: `projection.go`, internal key `chatto/search/checkpoint`]. The
replacement projector uses its own ordered consumer [verified: projectors use
ephemeral ordered consumers in `pkg/events/projector.go`]. After a restart,
the provider finds the pointer, opens the serving generation, and resumes the
replacement generation from its own checkpoint. At most one replacement
generation can exist.

Existing installations have the index at the root of the configured
directory [verified: `validateRebuildDirectory` accepts only
`index_meta.json`, `store`, the lock file, and the rebuild marker]. The
provider treats a root index without a pointer file as the legacy generation.
The first side-by-side rebuild writes the new generation below the root. After
the switch, the provider removes the legacy root entries.

**Tradeoff:** The directory layout changes. An older Chatto release that opens
a directory after a switch does not find a root index. It fails closed with
the existing recovery instructions [verified: `open` refuses a directory with
unknown entries]. It does not delete data. Downgrade after a switch therefore
needs a manual index rebuild.

### 3. Validate with a real query before the switch

**Decision:** Before the switch, run one bounded query through the
replacement index's normal read path. Use the same probe mechanism that [#3662](https://github.com/chattocorp/chatto/issues/3662)
adds. The probe must not use message text or terms from members. A filter-only
query, for example a page of one visible message, is sufficient. If the
replacement replay indexed at least one visible message, the probe must
return at least one hit. A probe failure or a timeout keeps the serving index.
The provider can try the validation again later.
**Why:** The incident in [#3659](https://github.com/chattocorp/chatto/issues/3659) showed that a status flag can report ready
while the index does not work. Only a real read proves that the new index can
answer. A filter-only query does not put private data in logs or metrics.
**Tradeoff:** This probe proves that the read path works. It does not prove
that the index is complete. A comparison of document counts between the two
generations is not reliable, because a format change can change which
messages the index contains (for example, echo handling in contract v12).

### 4. The serving index stays live, and its results report staleness

**Decision:** The serving index continues to apply live events with its own
projector while the replacement builds. Each provider query response reports
the freshness of the index that answered it. The provider reports "stale"
when the serving index has not completed its tail replay after a restart, or
when [#3660](https://github.com/chattocorp/chatto/issues/3660) reports that its projector is stalled. Normal live lag of some
milliseconds does not count as stale. Chatto copies the worst freshness value
of the provider pages that it used into the public response.
**Why:** A live serving index keeps retractions and crypto-shredding effective
in the provider index during a long rebuild. A frozen index would keep that
plaintext searchable by the provider for the full rebuild. Chatto already
removes such hits before delivery [verified: `HydrateHits` drops hits whose
body revision is not current, retracted, or not readable; its comment states
that it also drops crypto-shredded candidates], but a live index is the better
privacy default (FDR-033, decision 5). A stalled serving index is the
incident case: it must continue to answer, but it must say that it is stale.

For a known format change, the serving index has an older contract. Bleve
stores the index mapping in the index, so new documents use the old mapping
[inferred]. The provider then reports `DEGRADED`, because recall can differ
from the new format. The implementation must test live writes with the
current code into an index of the previous contract. If a future contract
cannot accept these writes, that contract change must use the rebuild in
place that exists today.

**Tradeoff:** The provider runs two projectors during a rebuild. This doubles
indexing CPU, EVT reads, and DEK unwrap calls for that time.

### 5. No partial results from a fresh index in the first implementation

**Decision:** A provider without a usable serving index does not answer
queries. It reports `INDEXING`, as today.
**Why:** An empty page from a partial index looks like "no matches". Every
client would need a new state to show this correctly. Old clients cannot show
it at all. Side-by-side serving already covers the common cases: upgrades with
a format change and explicit recovery.
**Tradeoff:** A new installation, or a server with an unreadable index, has no
Search until the first replay completes. If we add partial results later, the
response must mark them as incomplete with its own value (see the open
questions). An empty incomplete page must not show "no matches".

### 6. Remove the commit/query lock coupling before serving during a rebuild

**Decision:** Queries must not wait for index commits. Replace the one
`sync.RWMutex` with these two parts:

- A **lifecycle guard** that protects the index handle from close and
  replacement. Queries hold it in shared mode. Only close and the generation
  switch take it in exclusive mode. Commits do not take it.
- **Writer-owned state.** The projector already applies batches one at a time
  through one apply barrier (ADR-088). The DEK map and the checkpoint record
  belong to the writer. They do not need a lock that queries share.

Bleve indexes are safe for concurrent use, and the Scorch engine gives each
search a consistent snapshot while a batch commits [inferred from Bleve
documentation]. The implementation must prove this with a test that blocks a
commit and runs queries at the same time.
**Why:** Today `applyBatch` holds the exclusive lock across event preparation,
DEK reads from `RUNTIME_STATE`, KMS key unwrap, and the Bleve commit
[verified: `projection.go` and `crypto.go`]. So a slow NATS read, a slow KMS
call, or a blocked commit stops all queries. The backend rules also say: do
not hold the apply barrier across NATS or other external I/O (`cli/AGENTS.md`).
Other options are weaker:

- Shorter critical sections, with decryption outside the lock, help, but a
  blocked Bleve commit still blocks queries.
- A query timeout alone (from [#3662](https://github.com/chattocorp/chatto/issues/3662)) limits the damage, but queries still fail
  while a commit runs.

**Tradeoff:** If Bleve blocks readers inside its own commit path, this change
does not help. The [#3662](https://github.com/chattocorp/chatto/issues/3662) query probe then detects the problem, and the
provider must report it.

### 7. Check disk space before a side-by-side rebuild

**Decision:** Before the provider starts a replacement build, it compares the
free space on the index volume with the size of the serving generation plus a
margin. The initial margin is the same size again. If there is not sufficient
space, the provider does not start the side-by-side build:

- For an explicit operator recovery, the provider refuses and reports the
  required space.
- For an automatic format-change rebuild, the provider uses the rebuild in
  place that exists today, logs a warning, and reports `INDEXING`.

If the volume becomes full during the build, the provider stops the build,
deletes only the replacement generation, and continues to serve.
**Why:** Two indexes need about two times the disk space during the build,
plus space for Bleve segment merges [inferred]. The fallback keeps upgrades
automatic and keeps today's behavior on volumes that are too small.
**Tradeoff:** Operators must size the search volume for two indexes to get
search during rebuilds. Free-space checks are platform-specific. The fallback
to a rebuild in place still makes Search unavailable on small volumes.

### 8. Remove the old generation after a successful switch

**Decision:** After an automatic rebuild, the provider deletes the old
generation when the switch is durable. After an explicit recovery, [#3663](https://github.com/chattocorp/chatto/issues/3663)
decides how long the provider keeps the previous index.
**Why:** The old generation contains plaintext-derived data. Physical removal
completes crypto-shredding and retractions on disk sooner (FDR-033, decision
5). The epic rule "do not delete an index automatically after a failed probe"
does not apply here: the provider deletes the old index only after a
successful validation and switch, never after a failure.
**Tradeoff:** Operators who want to inspect the old index after an automatic
rebuild must copy it first.

### 9. Each provider replica rebuilds and switches independently

**Decision:** Each provider process owns its own directory and decides its own
switch. Replicas in the shared query queue can answer from different
generations at the same time. Each response reports its own freshness.
**Why:** Provider cursors already work across compatible replicas and do not
pin a snapshot (ADR-055). Cross-replica coordination would need a new shared
state and would not improve correctness, because Chatto authorizes and checks
every hit.
**Tradeoff:** During a rolling rebuild, two pages of one search can come from
different generations. FDR-033 already allows this.

### 10. Protocol changes are additive

**Decision:** Add one freshness field to the provider `QueryResponse` and one
matching field to the public `SearchMessagesResponse`. Use enums with these
values: unspecified, current, and stale. Add no other wire changes.
**Why:** ADR-045 and `proto/AGENTS.md` allow additive fields. The
unspecified value has a defined meaning: the response came from a provider or
server that does not report freshness. Such providers answer only from a
completed replay [verified: `Provider.Query`], so clients treat unspecified
as current. Old clients ignore the new field and show results without the
notice. Old servers never send the field. New clients connected to an old
server therefore show no notice, which is correct for those servers. The
minimum connectable server version does not change. The existing status
states keep their meaning: a provider that serves during a rebuild reports
`READY` (current serving index, same contract) or `DEGRADED` (stale serving
index or older contract), and joins the ready status queue. Rebuild progress
is operator telemetry from [#3660](https://github.com/chattocorp/chatto/issues/3660) and [#3661](https://github.com/chattocorp/chatto/issues/3661), not a public field.
**Tradeoff:** The value "unspecified means current" depends on the old
provider contract. A third-party provider that answers during its own
catch-up and does not set the field looks current. The provider contract
documentation must tell provider authors to set the field.

## Operations And Deployment

- **Embedded and standalone providers** use the same unit and the same
  behavior (ADR-041, ADR-055). Status aggregation across providers belongs to
  [#3661](https://github.com/chattocorp/chatto/issues/3661).
- **Readiness:** A provider that serves from a serving index is ready during a
  rebuild. A long rebuild therefore does not cause readiness failures or
  restart loops. Process health, serving readiness, and the NATS subscription
  alignment belong to [#3662](https://github.com/chattocorp/chatto/issues/3662) and [chattocorp/chatto-operator#89](https://github.com/chattocorp/chatto-operator/issues/89).
- **Restart during a rebuild:** The serving generation and the replacement
  generation both resume from their own checkpoints. A configuration rollout
  during a rebuild, as seen in the incident, does not lose replay progress.
- **Operator volume size:** The operator should document or check that the
  search volume can hold two indexes. The explicit rebuild trigger belongs to
  [#3663](https://github.com/chattocorp/chatto/issues/3663) and [chattocorp/chatto-operator#89](https://github.com/chattocorp/chatto-operator/issues/89).
- **Documentation:** When this feature ships, update the search operations
  page on the docs website with the volume-size need, the new directory
  layout, and the downgrade note. Update `docs/architecture/projections.md`
  and `docs/architecture/runtime-components.md` for the second projector,
  the generation layout, and the lock change.

## Related

- **Issues:** [#3665](https://github.com/chattocorp/chatto/issues/3665) (this design), epic [#3666](https://github.com/chattocorp/chatto/issues/3666), incident [#3659](https://github.com/chattocorp/chatto/issues/3659), [#3660](https://github.com/chattocorp/chatto/issues/3660) (stall
  detection and freshness), [#3661](https://github.com/chattocorp/chatto/issues/3661) (Admin search health), [#3662](https://github.com/chattocorp/chatto/issues/3662) (diagnostics
  and query probe), [#3663](https://github.com/chattocorp/chatto/issues/3663) (explicit recovery), [chattocorp/chatto-operator#89](https://github.com/chattocorp/chatto-operator/issues/89).
- **ADRs:** ADR-041 (runtime units), ADR-045 (public API stability tiers),
  ADR-053 (versioned NATS service namespaces), ADR-054 (optional projection
  persistence), ADR-055 (pluggable message search over NATS), ADR-088
  (componentized projections behind one apply barrier).
- **FDRs:** FDR-033 (Message Search), FDR-044 (My Threads).

## Open Questions

1. **Live writes to an older contract.** Can each known format change accept
   live writes with the current code? If not, which contract changes must use
   the rebuild in place? (Decision 4.)
2. **Stale threshold.** Which [#3660](https://github.com/chattocorp/chatto/issues/3660) signal starts the stale value, and after
   which delay? A short delay prevents flapping; a long delay hides real lag.
3. **Disk margin.** Is "two times the serving index" a sufficient margin for
   Bleve merges on real data? Measure on a restored production-size history.
4. **Fallback for automatic rebuilds.** When disk space is not sufficient,
   should the provider fall back to a rebuild in place (recommended), or
   refuse and wait for an operator?
5. **Stronger validation.** Is a filter-only probe sufficient, or must the
   switch also compare a sample of message IDs between the two generations?
6. **Partial results.** Do we ever want partial results from a fresh index?
   If yes, add a separate "incomplete" freshness value. Clients must then show
   "Search is still indexing" instead of "no matches" for an empty page.
7. **Third-party providers.** Must the provider contract require the
   freshness field from a provider that answers during catch-up?
8. **Retention after recovery.** How long does [#3663](https://github.com/chattocorp/chatto/issues/3663) keep the previous index,
   and how does that interact with the two-index disk need?
