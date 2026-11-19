# ADR-015: Separate console activity metadata from paged output

**Status:** Accepted
**Date:** 2026-11-09

## Context

CI jobs produce much more output than ordinary workflow activity. Loading all
output into the browser and collecting it under every ancestor task increases
memory use and repeats formatting work whenever another line arrives.

## Decision

Keep the existing JSONL run journal as the authoritative history. Keep workflow
state snapshots unchanged under [ADR-010](ADR-010-workflow-state-snapshots.md).
The console uses a metadata view that excludes log events and buffered command
output. The server also retains this smaller view for active runs. The existing
full-detail API reads the journal when requested.

Output readers query bounded pages from the journal. Cursors identify output
entries across execution attempts and remain independent of activity filters.
A task query selects its descendants through activity ancestry. It does not
copy their output into parent buffers. Readers preserve journal order and real
repeated messages. Older journals need no conversion.

The live metadata stream sends coalesced output notifications. Readers fetch
after their last cursor. Reconnection refreshes metadata and resumes output
queries. The Svelte console uses Virtua for continuous scrollback, with stable
entry keys and measured row heights. It loads pages near either edge and evicts
text at the opposite edge. Prepends and head evictions use Virtua's scroll
anchoring in separate layout updates. Explicit jumps to the beginning or latest
output establish a new measurement origin. A streamed download provides the complete selected log with transport
backpressure and cancellation.

## Consequences

- Browser output memory depends on the cache limits, not total output or nesting
  depth. The cache retains at most 1,000 entries or 1 MiB of text, plus an incoming
  page during a layout update. Evicted output remains available through journal
  queries. Activity projections update separately from bulk output.
- Virtua renders visible rows and a small buffer. The console uses the same
  version and workspace patch as the Chatto frontend, with its own dependency.
  It does not import Chatto application code.
- Closed activity rows do not load output. Only the visible details pane or
  dialog creates its output reader.
- Reads capture a finite journal prefix and ignore an incomplete final record.
  A later read can include that record after its writer completes it.
- Pages contain at most 200 entries by default, with a 256 KiB text limit. The
  API allows up to 500 entries. Large legacy buffers use smaller entries.
- JSONL reads currently scan the captured journal prefix. Sparse indexes can
  reduce read work if measurements show a bottleneck. Compressed output chunks
  and separate output files remain future work.
- The full-detail API can still return large histories when explicitly requested.
  Activity metadata can also grow with the number of tasks and agent updates.
- Retention continues to remove the run journal and run-owned files together.
