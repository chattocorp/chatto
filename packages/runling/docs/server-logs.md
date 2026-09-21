# Server logs

Both the development server (`mise x -- pnpm --dir packages/runling dev`
from the monorepo root) and `runling serve` write JSON log lines to the console and
`.runling/logs/server.jsonl`, relative to the directory that contains the Runling
config file.

```sh
tail -f .runling/logs/server.jsonl
```

Logs include server startup and shutdown, HTTP response status and duration,
request and config reload errors, webhook deliveries handled without a new run, and run starts,
results, and recovery errors. Run records include `runId` so you can find the
full event history in `.runling/runs/<runId>.jsonl`.

HTTP logs use route templates and omit request bodies, headers, and query
strings. Error messages can contain workflow or service details. Log files are
private to the server user and ignored by Git.

At 10 MiB, the server moves the log to `server.jsonl.1`, replacing the previous
backup. File write failures go to the console and do not stop workflows.
For SSE requests, the duration measures response setup, not stream lifetime.
