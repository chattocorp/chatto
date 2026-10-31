# Import performance

Run `mise bench-import` to measure a fresh synthetic interchange import through
the CLI and the Operator API Unix socket. Set `CHATTO_BENCH_COUNT` to change the
number of independent samples (default: three). Keep logs and profiles under
`.context/bench/`. No Slack data, external API, or running dev server is needed.

The versioned fixture has ten channels, two users, 1,000 message snapshots,
20 join events, ten final membership snapshots, and 100 thread replies. Each body
has 264 ASCII bytes. The two
cases differ only in reactions: none, or two reaction memberships per message.
The CLI interprets the archive and calls resource-specific Operator API methods.
Adjacent new messages without files share a batch with their final reactions.
Standalone reaction transitions use reaction batches. Other operations keep
their individual requests. The fixture is unchanged when comparing these paths.

Each sample starts with empty Chatto streams and a new progress journal. The
timer includes archive validation, identity checks, resource creation, history,
final room reconciliation, and journal writes. It excludes fixture generation,
server startup, and final checks. The benchmark checks completion and committed
message counts. Use `-benchtime=1x` when running the Go benchmark directly; more
iterations would reuse the destination and would not measure a fresh import.

The output includes:

- `ns/op`: elapsed time for the complete import.
- `messages/s`: snapshot throughput, including all other import work.
- `METHOD/calls`, `METHOD/mean-us`, `METHOD/p95-us`: server handler call count,
  mean time, and sampled 95th percentile for each RPC method.
- `server-s`: sum of handler times, including checks, storage, and projection
  waits. Requests are sequential in the current importer.
- `outside-server-s`: total elapsed time minus handler time. This includes
  archive processing, transport, client decoding, and durable journal writes.
  It is not a measurement of journal sync time alone.

A separate journal benchmark compares immediate syncs with grouped checkpoints
for replayable history results. Each sample writes 1,000 new entries and syncs
completion. This isolates local checkpoint cost without server calls. The
importer still syncs identity and upload prerequisites immediately.

The harness uses the real core and file-backed embedded JetStream, with an
in-process NATS connection. It does not start the full server stack: search
indexing, browser connections, attachment processing, and downloads are absent.
Client and server share a process. Thus CPU and allocation profiles contain
both. These results identify regressions and RPC costs; they do not predict
the duration or memory use of a large production import.

Compare repeated runs on the same machine and Go version. Keep the fixture
version constant. Use a full dev instance before claiming a production speedup,
particularly for attachment or search changes. Bump the fixture version when
the event mix changes.

For CPU or syscall attribution, run one case with Go's profiling flags from
`cli/`:

```sh
mkdir -p ../.context/bench/import
mise x -- go test ./cmd -run '^$' \
  -bench '^BenchmarkOperatorImport/v1/reactions_2$' -benchtime=1x -count=1 \
  -timeout=2m -cpuprofile=../.context/bench/import/cpu.pprof \
  -o ../.context/bench/import/cmd.test
mise x -- go tool pprof -top -cum \
  ../.context/bench/import/cmd.test ../.context/bench/import/cpu.pprof
```

In a separate run, replace `-cpuprofile` with
`-trace=../.context/bench/import/trace.out`. Then extract syscall delay:

```sh
mise x -- go tool trace -pprof=syscall ../.context/bench/import/trace.out \
  > ../.context/bench/import/syscall.pprof
mise x -- go tool pprof -top -cum ../.context/bench/import/syscall.pprof
```

CPU samples omit time asleep in a syscall. The syscall profile can expose disk
costs, but does not necessarily capture every platform-specific sync call. Use
the separate journal benchmark for checkpoint wall time. Profiling adds
overhead; use unprofiled runs for comparisons.

To isolate search commit cost, run the disk-backed Bleve benchmark from `cli/`:

```sh
mise x -- go test ./internal/search/bleve -run '^$' \
  -bench '^BenchmarkIndexCommits$' -benchtime=1x -count=3 -timeout=5m
```

Both cases apply the same 1,000 message bodies and posting events. One commits
each event separately; the other commits up to 256 events per transaction.
The timer excludes index creation and close. Each case checks the document
count and final checkpoint. Run this without another import or test workload.
It measures commit cost, not the live projector's 10 ms buffering delay.
