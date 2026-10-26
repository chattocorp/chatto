# Server logs

Both the development server (`mise x -- pnpm --dir packages/runling dev`
from the monorepo root) and `runling serve` print colored logs to the terminal.
Structured JSON records go to `.runling/logs/server.jsonl`, relative to the
directory that contains the Runling config file. `NO_COLOR` disables terminal
colors.

Applications use Runling's logger instead of writing their own terminal output:

```ts
import { log, step } from 'runling';

log.info('Connected', { pending: 3 });
log.warn('Retry scheduled', { delayMs: 1000 });
await step('Read context', async () => {
  /* application work */
});
```

`log.info`, `log.warn`, `log.error`, `log.success`, and `log.debug` accept optional
structured fields. `log.withFields(fields, work)` attaches fields to logs within
an asynchronous operation. Concurrent operations keep separate fields. Fields
must be host-owned metadata, never credentials, message text, or raw external
errors. A step records its timing and failure; its error text enters the journal,
so applications that handle private errors must supply a safe error at that boundary.

Source logs use a `[source:name]` prefix and enter the server log. Task logs use
run and task prefixes and enter the run journal. Both use the same renderer.
The console displays structured fields beside the message. The server prints
each root start once, without a second run-start message.

By default, the server terminal shows each run's detailed log: agent reasoning,
tool use, agent messages, and the errors of failed tasks. `runling run` also shows
run details, but keeps intermediate agent replies and task updates in its journal.
It prints the root task's final output once at the end, without indentation.

```text
10:32:01 [bright-waves-1234] ● Chat
10:32:02 [bright-waves-1234 / worker:task-2] ● [ripe-apes-1855] Agent started (model: provider/model)
10:32:05 [bright-waves-1234 / worker:task-2] ● [ripe-apes-1855] Thinking **Finding the composer**
10:32:06 [bright-waves-1234 / worker:task-2] ● [ripe-apes-1855] Reading src/composer.ts
```

This output can contain user messages and model text, including personal data.
It is not written to the log file. Where the terminal output is kept, for
example by Docker or journald, start the server with `--quiet-log`. The terminal
then shows only the safe activity summaries that the log file also holds:

```text
10:32:01 [bright-waves-1234 / task-1] ● Task started
10:32:02 [bright-waves-1234 / worker:task-2] ● Agent started · provider/model
10:32:32 [bright-waves-1234 / worker:task-2] ● Activity · 12 read, 8 search
```

With detailed output, routine summaries go only to the log file. Warnings and
errors go to both.

Prefixes identify the workflow run and task number, with the agent's optional
static `label`. Task numbers follow first observation and
remain stable within the live run. Structured records retain `agentId` and the
full `taskId` for journal lookup.
Task numbers are terminal labels, not separate runs or persistent references.

```sh
tail -f .runling/logs/server.jsonl
```

Logs include server startup and shutdown, failed HTTP response status and duration,
request and config reload errors, webhook deliveries handled without a new run, and run starts,
results, and recovery errors. Live runs also log task starts and finishes,
agent starts and outcomes, tool and command activity, and waits for messages.
A wait appears as `Waiting for a message`, and its end as `Message received` or
`Input closed`. These records do not contain the message.
Successful tool calls are counted and summarized at most every 30 seconds per
agent, with remaining counts shown at completion. Tool failures appear immediately
with the registered tool name. Individual starts and generic activity messages
are omitted. After two minutes without a recorded event from a running agent,
the terminal reports the quiet time. Further reminders are at least five
minutes apart. These reminders report silence, not progress. Run records include
`runId` so you can find the full event history in `.runling/runs/<runId>.jsonl`.

HTTP logs use route templates and omit request bodies, headers, and query
strings. Set `RUNLING_LOG_LEVEL=debug` to enable debug logs for server diagnostics,
sources, and workflows, including successful HTTP responses. `log.level = 'debug'`
also enables debug logging in that runtime. A workflow's `--verbose` option enables
debug for that execution without changing other executions.
Error messages can contain workflow or service details. Log files are
private to the server user and ignored by Git.

Activity records omit prompts, task labels, command text, tool arguments, message
contents, and results. They describe activity, not its content. Full execution
details remain in the run journal. A failed task appears as `Task failed`; its
error message is only in the journal, the console's Log view, and the detailed
terminal output. Agent labels must be static, host-owned text, never user data
or model output. Loading historical runs does not replay their
activity into the terminal. Explicit user cancellation is shown as cancelled,
even if a workflow handles its abort signal and returns normally. Server shutdown
is shown as interrupted. Runs with saved workflow state can resume under the same
reference after restart. Their journals retain each execution attempt; their
usage totals include all attempts. Runs without recoverable state remain history.

At 10 MiB, the server moves the log to `server.jsonl.1`, replacing the previous
backup. File write failures go to the console and do not stop workflows.
For SSE requests, the duration measures response setup, not stream lifetime.
