# Workflow messages and output

The host is the parent of the top-level task of a workflow run. The host can
send messages to the task while it runs, and it receives the messages that the
task emits. The task uses the same `ctx.inbox` and `ctx.emit()` as for a parent
task. See [task invocations and messages](task-channels.md).

## Output

The result of a run is the output of its top-level task. Runling validates the
output with the output schema of the task. It does not change the shape of the
output.

- `runling run` prints the output at the end. On a terminal, it shows a string
  as Markdown. It shows other values as JSON. With `--json`, it prints one JSON
  document: `{ durationMs, usage, output, error, ok }`.
- The web console shows the output in the run details. `GET /api/runs/:id`
  returns it.
- `runWorkflow()` returns `{ durationMs, usage, output, error, ok }`. When the
  run fails, `output` is `null` and `error` contains the error message.

## Send messages to a run

Each message must agree with the `receives` schema of the top-level task. If
the task has no `receives` schema, it accepts any message.

- **CLI.** On an interactive terminal, `runling run` sends each line that you
  type as a string message. If the task cannot accept a message, the log shows
  an error, and Runling discards the message. When standard input closes, the
  input of the task closes. Without a terminal, the input is closed from the
  start.
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

- `runling run` writes each update of the top-level task to its log. It writes
  a string as it is and other values as JSON.
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
reads the answer from `ctx.inbox`.
