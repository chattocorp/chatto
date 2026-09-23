# Agent steering

Use `agent.steer(text)` to send plain text into a running `run()` or `runOutcome()`
interaction. Steering does not expand slash commands or prompt templates.

```ts
const interaction = planner.runOutcome(ctx, "Investigate this change");
const delivered = planner.steer("Keep the existing public API");
const result = await interaction;
if (!await delivered) {
  // Keep the message for the next interaction.
}
```

The returned promise resolves to `true` when the message enters the agent's
conversation, before its next model call. This does not guarantee that the model
will follow the instruction. Steering cannot interrupt a response mid-generation
or a running tool. Do not await delivery from an agent event callback that must
return before the agent can continue.

The promise resolves to `false` if the agent is idle, disposed, cancelled, or the
interaction ends before delivery. Undelivered messages do not carry into another
interaction or fork. Keep them in the caller's inbox if they are still needed.
Delivery invalidates an earlier outcome report, so the agent must report again
after receiving steering. Token usage stays in the active interaction's context.

Use `runOutcome(ctx, prompt, { onText })` (or `run`) to observe completed assistant
text messages while the interaction runs. This callback excludes reasoning and
tool output. It is synchronous; callers that start asynchronous sends must handle
and await those sends themselves.

See the [Chatto plan example](../examples/chatto-plan-demo.md).
