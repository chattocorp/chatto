---
name: "chatto-debugging"
description: "Production debugging tools for NATS streams, KV buckets, and protobuf events. Covers nats CLI commands, stream inspection, protobuf decoding, and message iteration."
---

# Debugging

## Local Startup And Heap Profiling

When benchmarking projection startup or runtime memory locally, prefer the
built-in diagnostics knobs before adding ad hoc instrumentation:

```bash
CHATTO_METRICS_ENABLED=true \
CHATTO_METRICS_PPROF=true \
CHATTO_METRICS_BIND_ADDRESS=127.0.0.1 \
CHATTO_METRICS_PORT=9090 \
CHATTO_DIAGNOSTICS_STARTUP_CPU_PROFILE=.context/bench/startup.pprof \
mise x -- go run -tags bootstrap . run
```

- `CHATTO_DIAGNOSTICS_STARTUP_CPU_PROFILE` writes a Go CPU profile from early
  process startup through `ChattoCore.WaitForBoot`, which captures embedded
  NATS startup and projection replay.
- `CHATTO_METRICS_PPROF=true` exposes `/debug/pprof/` on the private metrics
  listener for heap, goroutine, trace, and under-load CPU profiles after boot.
- Keep benchmark data and profiles under `.context/bench/` so they remain
  gitignored and easy to compare across runs.
- Use `go tool pprof -top ./bin/chatto .context/bench/startup.pprof` or
  `go tool pprof -http=:0 ...` to inspect profiles. For heap profiles, fetch
  `http://127.0.0.1:9090/debug/pprof/heap` from the metrics port.
- When a large imported instance is available in `cli/data/`, build comparable
  binaries for `origin/main` and the branch with `go build -tags bootstrap`,
  copy `cli/data/` into a fresh `.context/bench/.../data` directory for each
  run, and alternate runs between revisions. On fast Apple Silicon, absolute
  startup times can understate Linux cluster costs; use repeated runs, pprof
  deltas, projection startup metrics, and retained heap profiles to identify
  the next target.
- For projection replay work, inspect allocation and CPU profiles before
  changing code. Past wins came from removing duplicate consumer fanout,
  duplicate protobuf decode, repeated subject matching, and repeated
  JetStream metadata parsing while preserving stable stream sequence semantics.

## NATS CLI for Production Debugging

This is the **only supported path** for raw NATS inspection. The ConnectRPC admin diagnostics surface intentionally exposes only aggregate operational metrics — it does not expose stream subjects, KV keys, or per-stream/per-bucket breakdowns. Those leak structural information (room IDs, user IDs, bucket names) without a use case the `nats` CLI doesn't already cover. If you need to inspect a specific stream or bucket, shell into the host with operator credentials and use the commands below.

The `nats` CLI can directly inspect streams and KV buckets to debug production issues. Useful commands:

```bash
# List all streams and their message counts
nats stream ls

# List subjects in a stream with message counts per subject
nats stream subjects EVT

# Get stream info (config, state, consumer count)
nats stream info EVT --json

# Get a specific message by sequence number
nats stream get EVT 51 --json

# Get the last message for a specific subject
nats stream get EVT -S "evt.room.{roomId}.message_posted" --json

# List all KV buckets
nats kv ls

# List keys in a KV bucket
nats kv ls RUNTIME_STATE

# Get a KV entry
nats kv get RUNTIME_STATE "session.{sessionId}"
```

### Decoding Protobuf Events

Stream messages are base64-encoded protobuf. To decode:

```bash
# Get message and decode raw protobuf (shows field numbers and values)
nats stream get STREAM_NAME SEQ --json | jq -r '.data' | base64 -d | protoc --decode_raw

# Common field numbers in SpaceRoomEvent:
# - 300: RoomCreatedEvent
# - 310: UserJoinedRoomEvent
# - 311: UserLeftRoomEvent
# - Field 1: event ID
# - Field 2: timestamp
# - Field 3: actor ID
```

### Iterating Through Stream Messages

To find all messages matching a pattern:

```bash
for i in $(seq 1 100); do
  result=$(nats stream get STREAM_NAME $i --json 2>/dev/null)
  if echo "$result" | jq -e '.subject | contains("pattern")' > /dev/null 2>&1; then
    echo "=== Seq $i ==="
    echo "$result" | jq -r '.subject, .time'
  fi
done
```
