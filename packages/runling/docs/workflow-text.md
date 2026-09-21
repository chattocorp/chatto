# Workflow messages

Tasks can send user-facing text through an optional context handler:

```ts
const implement = task(async ctx => {
  await ctx.onText?.("Running checks.");
});

await runWorkflow(implement, {
  input: undefined,
  onText: async text => postToThread(text),
});
```

Nested tasks inherit the handler through the context they receive. A parent can
compose a handler without changing the shared context:

```ts
await child({
  ...ctx,
  onText: async text => { await ctx.onText?.(`Implementation: ${text}`); },
});
```

Handlers can be synchronous or asynchronous. Await delivery when order matters;
a rejected handler fails the calling task unless the task catches the error.
Parallel tasks can call the handler concurrently. The host can queue delivery when order matters.

Text is not forwarded from agents automatically. The caller chooses which agent
messages belong in the conversation. This callback does not replace logs, task
results, or the run journal, and does not add a journal event by itself.
