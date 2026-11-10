/** A local container pipeline: own a service and run two independent checks concurrently. */
import { task, Type } from 'runling';
import { docker } from 'runling/docker';

const image = 'alpine:3.22';
const Input = Type.Object({ network: Type.String() });

const testService = task(
  { name: 'Test service', input: Input, result: Type.Number() },
  async (ctx, { network }) => {
    const result = await docker.run(ctx, {
      image,
      network,
      timeoutMs: 10_000,
      command: [
        'sh',
        '-c',
        'set -eu; wget -qO- http://service:8080 | grep -qx ready; echo "Service check passed"'
      ]
    });
    return result.exitCode;
  }
);

const testTools = task(
  { name: 'Test build tools', input: Type.Null(), result: Type.Number() },
  async (ctx) => {
    const result = await docker.run(ctx, {
      image,
      timeoutMs: 10_000,
      command: [
        'sh',
        '-c',
        'set -eu; printf "Checking tools\\n"; test "$(printf hello | wc -c)" -eq 5; sleep 1; echo "Tool check passed"'
      ]
    });
    return result.exitCode;
  }
);

export default task(
  {
    name: 'Docker pipeline',
    input: Type.Object({}),
    result: Type.Object({ checks: Type.Number() })
  },
  async (ctx) => {
    await using network = await docker.network(ctx);
    await using service = await docker.start(ctx, {
      image,
      network: network.name,
      networkAliases: ['service'],
      // This fixture uses only tools included in Alpine. Discard client requests so service
      // logs contain no request headers. A real service would use its own image and probe.
      command: [
        'sh',
        '-c',
        'while true; do printf "HTTP/1.1 200 OK\\r\\nContent-Length: 6\\r\\nConnection: close\\r\\n\\r\\nready\\n" | nc -l -p 8080 >/dev/null; done'
      ],
      ready: { command: ['wget', '-q', '-O', '/dev/null', 'http://127.0.0.1:8080'] }
    });
    // Invocation disposal cancels unfinished siblings if a check fails. Dispose checks before
    // the service and network that they use. No second scheduler is needed.
    await using serviceCheck = ctx.start(testService, { network: network.name });
    await using toolsCheck = ctx.start(testTools, null);
    await Promise.all([serviceCheck.result, toolsCheck.result]);
    return { checks: 2 };
  }
);
