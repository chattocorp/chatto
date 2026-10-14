# Agents

Import the agent APIs from `runling`: `agent`, `runAgent`,
`defineAgentExtension`, `taskTool`, and their types.

- `agent(options)` creates a Pi agent session. Call `run()` or `runOutcome()`
  for each turn. Dispose of the agent when you do not need it.
- `runAgent(ctx, prompt, options)` creates an agent, runs one turn, and disposes
  of the agent.

```ts
import { runAgent } from 'runling';

const report = await runAgent(ctx, 'Find the cause of the failing test', {
  cwd: directory,
  model: 'openai-codex/gpt-5.6-sol',
  tools: ['read', 'grep', 'find', 'ls']
});
```

## Output modes

The default, `output: "report"`, is for coding and specialist tasks. The agent
must call `report_outcome` with an outcome (`completed`, `blocked`, or
`failed`), a one-line `summary`, and optional `details`. If the agent does not
report, Runling asks it one more time.

`output: "text"` is for conversational agents. The agent finishes its turn
naturally. Runling does not add `report_outcome` or its report instructions.
The result is `completed`, and `summary` contains the final text of the agent.
An empty response or a provider error gives a `failed` outcome.

- `run()` throws `AgentOutcomeError` when the outcome is not `completed`.
- `runOutcome()` returns every outcome. A result can have a `failureReason`
  from the host: `provider_error` when the model provider failed, or
  `missing_outcome` when report mode got no valid report. These are different
  from a `blocked` outcome that the model reports. Use the reason. Do not parse
  the summary.

A recognized expired-login error gives a safe summary that tells the operator
to sign in again on the agent host. The summary does not contain the raw
provider error.

Set `systemPrompt` to replace Pi's default coding-agent prompt, for example for
a chat assistant. `instructions` add more instructions. In report mode, Runling
also adds its outcome rules.

Use `run(ctx, prompt, { onText })` or `runOutcome()` with `onText` to see each
completed assistant text message during a turn. The callback does not get
reasoning or tool output.

## An agent as a task

An agent is the implementation of a task. The task receives messages from its
parent and emits the replies of the agent. This conversational task answers each
message from its parent:

```ts
import { agent, task, Type } from 'runling';

export default task(
  {
    name: 'Chat',
    input: Type.String(),
    output: Type.String(),
    receives: Type.String(),
    emits: Type.String()
  },
  async (ctx, message) => {
    await using bot = await agent({ cwd: '.', model, output: 'text' });
    const inbox = ctx.inbox[Symbol.asyncIterator]();
    while (true) {
      const { summary } = await bot.run(ctx, message);
      await ctx.emit(summary);
      const next = await inbox.next();
      if (next.done) return summary;
      message = next.value;
    }
  }
);
```

Run it with `runling run chat.ts "Hello"` and type the next messages. A message
that arrives during a turn waits in the inbox until the turn ends. To give the
message to the agent during the turn, use [steering](agent-steering.md).

## Continue a conversation

By default, an agent keeps its conversation in memory, and the conversation ends
with the process. Set `sessionFile` to a path to keep it in a JSONL file
instead. When the file exists, a new agent with the same `sessionFile`
continues the conversation with its full context, for example after a
cancellation or a restart. When the file does not exist, the agent starts a new
conversation in it. Pi writes the file after the first model reply.

```ts
const worker = await agent({ model, cwd, sessionFile: join(artifactDirectory, 'worker.jsonl') });
```

The file contains the complete model context: prompts, tool results, and
replies. Keep it private, as other workflow data. `sessionFile` cannot be
combined with `trust`, because Runling keeps the untrusted mark only in memory.

`agent.fork()` creates an independent agent in memory with a copy of the
conversation. A fork does not use the session file of its source.

## Untrusted content

Set `trust` when an agent can read content that other people control, such as web
pages, and also has tools that must not act on that content:

```ts
await using owner = await agent({
  cwd: '.',
  model,
  tools: ['researchWeb', 'publishChange'],
  trust: { untrusted: ['researchWeb'], blockAfterUntrusted: ['publishChange'] }
});
```

After a result from an `untrusted` tool, Runling blocks the tools in
`blockAfterUntrusted` for the remaining lifetime of the agent and its forks.
The tool does not run. The model gets an error result with a fixed reason. Set
`onBlocked` to run host code first, for example to post a fixed message to the
user. User messages do not remove the block, because the content stays in the
history. Start a new agent to use the blocked tools again. Prefer separate
agents for untrusted input and powerful actions, and use `trust` as a second
protection.

The option installs the Pi extension from `runling/extensions/trust`. Plain Pi
projects can use it through `createTrustExtension(policy).extension`. See
[ADR-005](adr/ADR-005-untrusted-context.md) and
[FDR-006](fdr/FDR-006-untrusted-tool-blocking.md).

## Codemode

Set `codemode` to let the model write a JavaScript script that calls the tools
of the agent. Only the output of the script reaches the model. Thus a script can
run several calls in parallel and filter large results first:

```ts
await using owner = await agent({
  cwd: '.',
  model,
  tools: ['readIssue', 'searchIssues'],
  codemode: true // or { mode: 'only' } to reach the other tools only through scripts
});
```

Scripts run in Pi's QuickJS sandbox, without Node APIs, files, network, or
timers. They can call the tools in `tools`, but not `report_outcome` or Pi's
`models` API. A script stops after `codemode.timeoutMs` (ten minutes by default)
and can make at most `codemode.maxCalls` tool calls (100 by default). Scripts
get only the text of tool results. Register tools that return images or end the
turn with `exposure: 'model-only'`. Pi saves the complete output of a long
script to a temporary file. Runling deletes the file when the agent ends, or at
once when the agent has no `read` tool. Set `TMPDIR` to a private directory when
tool results are sensitive. Each call from a script goes through the same hooks
as a call from the model, so `trust` and other extension gates apply. See
[FDR-008](fdr/FDR-008-codemode.md).

## Tasks as agent tools

`taskTool(ctx, task, { name, description, label? })` gives a task to an agent as
a tool. Create the tool inside the task that runs the agent, so the tool uses
the context of that task:

```ts
import { agent, defineAgentExtension, task, taskTool, Type } from 'runling';

const greet = task(
  { name: 'Greet', input: Type.Object({ name: Type.String() }), output: Type.String() },
  async (_ctx, { name }) => `Hello, ${name}`
);

export default task(
  { name: 'Greeter', input: Type.Null(), output: Type.String() },
  async (ctx) => {
    const tools = defineAgentExtension((pi) => {
      pi.registerTool(
        taskTool(ctx, greet, { name: 'greet', description: 'Return a greeting for a name.' })
      );
    });

    await using worker = await agent({ cwd: '.', model, tools: ['greet'], extensions: [tools] });
    return (await worker.run(ctx, 'Greet Ada.')).summary;
  }
);
```

- The tool parameters are the input schema of the task. The input schema must
  be a TypeBox schema. Else `taskTool` throws a `TypeError`.
- Each call starts the task as a child of `ctx`, closes its input, and waits for
  its output. The model gets the output as text: a string as it is, other values
  as JSON.
- The updates of the task are partial tool results.
- Cancellation of the tool call cancels the task. Cancellation of `ctx` also
  cancels it.
- The task validates its input and output. An error of the task is the error of
  the tool call.
- `label` is a short name for logs. The default is the task name.

A parent that waits for a long child does not read its own inbox during the
wait, unless its code does so. Decide what to do with new messages, for example
send them to the child or reply that the work continues.
