# Agent steering

For an agent task, use `ctx.start` and send text through the invocation:

```ts
await using work = ctx.start(planner, 'Investigate this change');
await work.steer('Keep the existing public API');
for await (const reply of work.output) console.log(reply);
const result = await work.result;
```

Runling forwards messages to the session while it runs. If a message that it
read cannot enter the current turn, it starts another turn with that message.
Steering closes when the task finishes. The task does not wait for more work after
its current work ends. A successful `steer` means queued, not processed.

## Explicit session steering

For a session created with `createAgentSession()`, use `session.steer(text)` to
send plain text into a running `run()` or `runOutcome()` interaction. Steering
does not expand slash commands or prompt templates.

```ts
const interaction = planner.runOutcome(ctx, 'Investigate this change');
const delivered = planner.steer('Keep the existing public API');
const result = await interaction;
if (!(await delivered)) {
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

Use `runOutcome(ctx, prompt, { onText })` (or `run`) to see completed assistant
text messages while the interaction runs. This callback does not get reasoning
or tool output. It is synchronous. If it starts an asynchronous send, the caller
must handle and wait for that send. `runling run` records these messages in
the journal and prints only the root task's final output.

## Steer from a task inbox

The `agent()` task handles this forwarding. For a custom session loop, read
`ctx.steering` while the turn runs, and call `steer()` for each message. Keep each
message for which `steer()` resolves to `false`, and use it in the next turn.
`ctx.steering` has one reader only, so the same code must read the inbox for all
turns. A failed or cancelled task makes the inbox reader reject; handle that
rejection. See [agents](agents.md) and
[task invocations and messages](task-channels.md).
