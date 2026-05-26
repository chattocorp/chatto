---
name: "chatto-debugging"
description: "Production debugging tools for NATS streams, KV buckets, and protobuf events. Covers nats CLI commands, stream inspection, protobuf decoding, and message iteration."
---

# Debugging

## NATS CLI for Production Debugging

This is the **only supported path** for raw NATS inspection. The GraphQL admin surface intentionally exposes only aggregate operational metrics (`Query.admin.systemInfo` returns connection state and account-level usage totals) — it does not expose stream subjects, KV keys, or per-stream/per-bucket breakdowns. Those leaked structural information (room IDs, user IDs, bucket names) without a use case the `nats` CLI doesn't already cover. If you need to inspect a specific stream or bucket, shell into the host with operator credentials and use the commands below.

The `nats` CLI can directly inspect streams and KV buckets to debug production issues. Useful commands:

```bash
# List all streams and their message counts
nats stream ls

# List subjects in a stream with message counts per subject
nats stream subjects SPACE_{spaceId}_EVENTS

# Get stream info (config, state, consumer count)
nats stream info SPACE_{spaceId}_EVENTS --json

# Get a specific message by sequence number
nats stream get SPACE_{spaceId}_EVENTS 51 --json

# Get the last message for a specific subject
nats stream get SPACE_{spaceId}_EVENTS -S "space.{spaceId}.room.{roomId}.meta" --json

# List all KV buckets
nats kv ls

# List keys in a KV bucket
nats kv ls SPACE_{spaceId}_CONFIG

# Get a KV entry
nats kv get SPACE_{spaceId}_CONFIG "room_membership.{userId}.{roomId}"
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
