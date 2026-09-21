# Incoming workflow messages

Use [task channels](task-channels.md) to send data to running tasks.
A parent calls `child.send(value)`; the child reads `ctx.inbox` and replies
through `ctx.emit(update)`.

For agents, [`connectAgent`](agents.md) forwards inbox messages to the active
interaction. Its delivery callback distinguishes queueing from consumption.
The sender must retain or reroute messages the agent did not consume.

See the [Chatto coordinator example](../examples/chatto-coordinator-demo.md).

This replaces `ctx.messages` and `createMessageChannel()`. Callers must migrate
to explicit task channels; `send()` now confirms queue acceptance, not consumption.
