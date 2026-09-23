# Server logs

Both the development server (`mise x -- pnpm --dir packages/runling dev`
from the monorepo root) and `runling serve` print colored, single-line activity
logs to the terminal. Structured JSON records go to `.runling/logs/server.jsonl`,
relative to the directory that contains the Runling config file. `NO_COLOR`
disables terminal colors.

```text
10:32:01 [bright-waves-1234] ● Started conversation
10:32:01 [bright-waves-1234 / task-1] ● Task started
10:32:02 [bright-waves-1234 / worker:task-2] ● Agent started · provider/model
10:32:32 [bright-waves-1234 / worker:task-2] ● Activity · 12 read, 8 search
10:32:40 [bright-waves-1234 / worker:task-2] ● Validating change
```

Prefixes identify the workflow run and task number, with the agent's optional
static `label`. Task numbers follow first observation and
remain stable within the live run. Structured records retain `agentId` and the
full `taskId` for journal lookup.
Task numbers are terminal labels, not separate runs or persistent references.

```sh
tail -f .runling/logs/server.jsonl
```

Logs include server startup and shutdown, HTTP response status and duration,
request and config reload errors, webhook deliveries handled without a new run, and run starts,
results, and recovery errors. Live runs also log task starts and finishes,
agent starts and outcomes, tool and command activity, and input waits and receipts.
Successful tool calls are counted and summarized at most every 30 seconds per
agent, with remaining counts shown at completion. Tool failures appear immediately
with the registered tool name. Individual starts and generic activity messages
are omitted. After two minutes without a recorded event from a running agent,
the terminal reports the quiet time. Further reminders are at least five
minutes apart. These reminders report silence, not progress. Run records include
`runId` so you can find the full event history in `.runling/runs/<runId>.jsonl`.

HTTP logs use route templates and omit request bodies, headers, and query
strings. Error messages can contain workflow or service details. Log files are
private to the server user and ignored by Git.

Activity records omit prompts, task labels, command text, tool arguments, message
contents, and results. They describe activity, not its content. Full execution
details remain in the run journal. An observed task can explicitly publish a
public operational message with `ctx.emit({ type: "state", value, activity })`.
Agent labels and these messages must be static, host-owned text, never user data
or model output. State values are not copied into operational logs.
An activity can set `activityLevel` to `info`, `success`, or `error` for its
terminal marker and web Log view.
Loading historical runs does not replay their
activity into the terminal. Explicit user cancellation is shown as cancelled,
even if a workflow handles its abort signal and returns normally. Server shutdown
is shown as interrupted. Those runs remain history only after restart; new inputs
start new runs with separate references and timelines.

At 10 MiB, the server moves the log to `server.jsonl.1`, replacing the previous
backup. File write failures go to the console and do not stop workflows.
For SSE requests, the duration measures response setup, not stream lifetime.
