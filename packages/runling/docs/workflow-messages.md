# Workflow messages and output

The host is the parent of the top-level task of a workflow run. The host can
send messages to the task while it runs, and it receives the messages that the
task emits. The task uses the same `ctx.steering` and `ctx.emit()` as for a parent
task. See [task invocations and messages](task-channels.md).

## Output

The result of a run is the final return value of its top-level task. Runling
validates it with the task's `result` schema. Streamed `output` is separate.
The run journal, HTTP API, and `runWorkflow()` response keep their existing
`output` field for the final result.

- `runling run` prints only the root task's output, once at the end, without
  a log prefix or task indentation, with a blank line before it.
  On a terminal, it shows a string
  as Markdown. It shows other values as JSON. With `--json`, it prints one JSON
  document: `{ durationMs, usage, output, error, ok }`. The journal and JSON
  document retain the output. Run details show the reference, task progress,
  tool activity, token usage, duration, and journal path. With `--json`, these
  details go to stderr. Child replies and updates stay in the journal.
  `--verbose` shows debug logs. Failures are
  reported on stderr with a nonzero exit code.
- The web console shows the output in the run details. `GET /api/runs/:id`
  returns it.
- `runWorkflow()` returns `{ durationMs, usage, output, error, ok }`. When the
  run fails, `output` is `null` and `error` contains the error message.

[FDR-011](fdr/FDR-011-cli-output.md) defines the CLI display requirements and
their rationale. For example, a task that checks a repository can show:

```text
● Runling starting run bright-waves-1234
  ● Check repository
    ● [calm-birds-5678] Using bash
    ● [calm-birds-5678] Executing: git status
    ● [calm-birds-5678] Token usage: in 100, out 20
● Total token usage: in 100, out 20
● Finished in 1.2s
● Run bright-waves-1234 journal: .runling/runs/<id>.jsonl

The working tree is clean.
```

The tool supplies the `Executing: git status` step label in this example.
Runling preserves its agent prefix. The final sentence is the root result;
it appears once after the run details.

## Send messages to a run

Each message must agree with the `steering` schema of the top-level task. If
the task has no `steering` schema, it accepts any message.

- **CLI.** On an interactive terminal, `runling run` sends each line that you
  type as a string message. If the task cannot accept a message, Runling shows
  and records an error, then discards the message. When standard input closes, the
  input of the task closes. Without a terminal, the input is closed from the
  start. Sending messages does not require a flag or an input hint.
- **Web console.** A running run has a message composer below its log. Text
  that is valid JSON is sent as a JSON value. Other text is sent as a string.
- **HTTP.** Send `POST /api/runs/:id/messages` with the body
  `{ "message": <json> }`. The response is `202` when the run accepts the
  message. It is `400` for an invalid message, `409` when the run is not
  running or does not accept more messages, and `503` when the run has too many
  unread messages.
- **Routers.** `ctx.start` in a router returns a run handle. Keep the handle to
  send more messages to the run later. See [webhook routing](webhook-routing.md).

```sh
curl http://localhost:5173/api/runs/<run-id>/messages \
  -H 'content-type: application/json' -d '{ "message": "Also check the tests" }'
```

A `202` response means that the message is in the queue of the task. It does
not mean that the task read the message.

## Updates from a run

- `runling run` records updates in the journal without printing them. Only
  the root task's final output is printed.
- The web console Log view shows each message between the host and the
  top-level task. The timeline shows the messages between tasks.
- `runWorkflow()` gives each update to its `onUpdate` callback. Without
  `onUpdate`, Runling records updates in the journal and then discards them.

## Wait for a message

When a task reads an empty inbox, Runling records a wait. The timeline shows
"Waiting for a message" until a message arrives or the input closes. When a run
waits for a message and has no other active work, the run list and the status
badge also show "Waiting for a message". A run can wait for a long time. Runs
have no default timeout.

## Run a workflow in code

```ts
import { runWorkflow } from 'runling';

const execution = await runWorkflow(plan, {
  input: { directory: '.', prompt: 'Add a dark mode' },
  messages: answersFromUser(), // AsyncIterable of messages for the top-level task
  onUpdate: (question) => showToUser(question)
});
if (!execution.ok) throw new Error(execution.error!);
```

When the `messages` iterable ends, the input of the task closes. Without
`messages`, the input is closed from the start. `runWorkflow` also accepts
`timeout`, `signal`, `onEvent`, and `run`. It does not print and does not change
process state.

See the [planning example](../examples/plan.ts). It emits each question and
reads the answer from `ctx.steering`.
