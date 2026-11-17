# Network recovery checks

Use these checks when changing request retries or connection warnings. See
[FDR-031](../fdr/FDR-031-client-server-compatibility-discovery.md) for the
behavior contract.

## Automated browser check

From `apps/frontend/`, run:

```sh
mise x -- pnpm exec playwright test e2e/network-recovery.test.ts --workers=1 --retries=0 --max-failures=1
```

In a Geordi workspace, set `E2E_BASE_PORT=$GEORDI_PORT` before the command.
Stop the development stack first so the test can use the workspace port.
The test fixture starts and stops its own server.

The tests use the real browser, server, generated API clients, and interface:

- Abort timeline HTTP requests while the realtime WebSocket stays open.
  Check the network error, automatic load attempts, and orange header warning.
  Allow requests again and check that the timeline returns without a click
  and the warning disappears.
- Add 400 ms latency, a 100,000 byte/s download limit, and a 50,000 byte/s
  upload limit. Abort the first two timeline requests. Check that the third
  request loads the timeline without a manual retry.
- Let the server accept a message, then discard its HTTP response. Check
  that the client does not repeat the write during the read retry window.

Request abortion gives a repeatable transport failure. It does not simulate
TCP packet loss, radio changes, or every browser's offline behavior. The
throttled case uses Chromium's network emulation. Do not describe these
checks as verification of all poor connections.

## Manual check

Start `mise dev` and sign in with a local development account. In browser
developer tools, enable network throttling and navigate to a room. A slow
request alone need not show a warning: the warning needs a failed attempt.

Block the room timeline API request, then reload the room. Keep realtime
available. Check that the header warns about the connection and the timeline
tries again without a click. Remove the block and check recovery. Do not
block only WebSocket traffic when testing HTTP recovery.

The first warning pulses. After three failed API reads in sequence, the
warning is orange. A successful API response clears that count; a successful
realtime connection does not. There is no red warning stage. Automatic
transport retries apply to reads, not writes. No new external service is
contacted for these checks or for connection health probes.

Stop the development stack when the check is complete.
