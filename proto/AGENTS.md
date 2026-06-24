# Instructions for Agents Working on Protobuf Files

Protobuf comments are the source for Chatto's generated API reference.

## Public API Documentation

For files under `chatto/api/v1`, write comments for API consumers, not Chatto maintainers. Describe what a client can call, what data it receives, and how important fields should be interpreted.

Every public service, RPC, message, enum, enum value, and important field should have a useful comment. Keep field comments concise enough for generated tables, and put longer behavior notes on RPCs or messages.

Good RPC comments explain:

- what state the call reads or changes
- which IDs the client must provide
- pagination, cursor, and anchor semantics
- whether the call is available before login
- notable response fields or edge cases a client must handle

Avoid visible maintainer workflow text such as "edit the proto", "run codegen", or "do not edit generated output" in comments that render into the public docs.

After changing public API comments, regenerate outputs with `mise codegen-proto` so the docs and generated clients stay in sync.

## Compatibility

Do not renumber existing fields or change field types in messages that may already be persisted or consumed by clients. Prefer additive schema changes.
