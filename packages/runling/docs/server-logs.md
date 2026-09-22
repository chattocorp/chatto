# Server logs

Both the development server (`mise x -- pnpm --dir packages/runling dev`
from the monorepo root) and `runling serve` print colored, single-line activity
logs to the terminal. Structured JSON records go to `.runling/logs/server.jsonl`,
relative to the directory that contains the Runling config file. `NO_COLOR`
disables terminal colors.

```text
10:32:01 [bright-waves-1234] ● Started conversation
10:32:01 [bright-waves-1234 / task-1] ● Task started
10:32:02 [bright-waves-1234 / task-2 / calm-foxes-2468] ● Agent started
10:32:03 [bright-waves-1234 / task-2 / calm-foxes-2468] ● Tool read started
10:32:04 [bright-waves-1234 / task-2 / calm-foxes-2468] ● Tool read succeeded
```

Prefixes identify the workflow run, the task within that run, and the agent when
applicable. Task numbers follow first observation and remain stable within the
live run. Structured records include the full `taskId` for journal lookup.
Task numbers are terminal labels, not separate runs or persistent references.

```sh
tail -f .runling/logs/server.jsonl
```

Logs include server startup and shutdown, HTTP response status and duration,
request and config reload errors, webhook deliveries handled without a new run, and run starts,
results, and recovery errors. Live runs also log task starts and finishes,
agent starts and outcomes, tool and command activity, and input waits and receipts.
Repeated generic agent activity is limited to one line per agent per 15 seconds;
tool lifecycle events and failures appear immediately. No synthetic heartbeat
claims progress while an agent is silent. Run records include `runId` so you can find the
full event history in `.runling/runs/<runId>.jsonl`.

HTTP logs use route templates and omit request bodies, headers, and query
strings. Error messages can contain workflow or service details. Log files are
private to the server user and ignored by Git.

Activity records omit prompts, task labels, command text, tool arguments, message
contents, and results. They describe activity, not its content. Full execution
details remain in the run journal. Loading historical runs does not replay their
activity into the terminal. Cancellation is shown as cancelled, not completed or
failed, even if a workflow handles its abort signal and returns normally.

At 10 MiB, the server moves the log to `server.jsonl.1`, replacing the previous
backup. File write failures go to the console and do not stop workflows.
For SSE requests, the duration measures response setup, not stream lifetime.
