/** Opt-in tests against a real Docker daemon, including OrbStack. All resources are scoped. */
import { execa } from 'execa';
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { afterAll, beforeAll, describe, expect, test } from 'vitest';
import { docker, DockerTimeoutError } from './docker.ts';
import { observeRunlingEvents, type RunlingEvent } from './events.ts';
import { createTaskContext } from './context.ts';

const image = 'alpine:3.22';
const runId = `docker-integration-${crypto.randomUUID()}`;
const ctx = { ...createTaskContext(), run: { id: runId, setTitle() {} } };
const buildTags = new Set<string>();

describe.skipIf(process.env.RUNLING_TEST_DOCKER !== '1')('Docker daemon', () => {
  beforeAll(async () => {
    await execa('docker', ['image', 'pull', image]);
  }, 120_000);

  afterAll(async () => {
    // An assertion failure must not leave integration resources on the user's daemon.
    const containers = await execa('docker', [
      'ps',
      '-aq',
      '--filter',
      `label=dev.runling.run=${runId}`
    ]);
    const ids = containers.stdout.split(/\s+/).filter(Boolean);
    if (ids.length) await execa('docker', ['rm', '-fv', ...ids]);
    const networks = await execa('docker', [
      'network',
      'ls',
      '-q',
      '--filter',
      `label=dev.runling.run=${runId}`
    ]);
    const names = networks.stdout.split(/\s+/).filter(Boolean);
    if (names.length) await execa('docker', ['network', 'rm', ...names]);
    expect(ids, 'Containers leaked before test cleanup').toEqual([]);
    expect(names, 'Networks leaked before test cleanup').toEqual([]);
    for (const tag of buildTags)
      await execa('docker', ['image', 'rm', '--force', tag], { reject: false });
  });

  test('builds an immutable image with live activity output and a temporary ID file owned by Runling', async () => {
    const directory = await mkdtemp(join(tmpdir(), 'runling-build-'));
    const tag = `runling-build-test-${crypto.randomUUID()}:local`;
    buildTags.add(tag);
    try {
      await writeFile(
        join(directory, 'Dockerfile'),
        'FROM scratch\nARG MARKER\nLABEL runling.test.marker=$MARKER\nCOPY input /input\n'
      );
      await writeFile(join(directory, 'input'), runId);
      const events: RunlingEvent[] = [];
      const output: string[] = [];
      let settled = false;
      const operation = observeRunlingEvents(
        (event) => events.push(event),
        () =>
          docker.build(ctx, {
            contextDirectory: directory,
            tag,
            buildArgs: { MARKER: runId },
            onOutput: ({ text }) => {
              expect(settled).toBe(false);
              output.push(text);
            }
          })
      );
      void operation.then(() => {
        settled = true;
      });
      const image = await operation;
      expect(image).toMatch(/^sha256:[a-f0-9]{64}$/);
      const inspected = await execa('docker', [
        'image',
        'inspect',
        '--format',
        '{{.Id}} {{index .Config.Labels "runling.test.marker"}}',
        tag
      ]);
      expect(inspected.stdout).toBe(`${image} ${runId}`);
      expect(output.join('')).not.toBe('');
      expect(events).toContainEqual(
        expect.objectContaining({ type: 'command.started', command: `Docker build ${tag}` })
      );
      expect(events).toContainEqual(expect.objectContaining({ type: 'log', source: 'command' }));
      expect(events.filter((event) => event.type === 'command.finished')).toEqual([
        expect.objectContaining({ status: 'completed', output: { stdout: '', stderr: '' } })
      ]);
    } finally {
      await rm(directory, { recursive: true, force: true });
    }
  }, 60_000);

  test('build failure, task cancellation, and output callback failure settle their command activities', async () => {
    const directory = await mkdtemp(join(tmpdir(), 'runling-build-failure-'));
    try {
      await writeFile(join(directory, 'Dockerfile'), 'FROM scratch\nCOPY absent-file /input\n');
      const failed: RunlingEvent[] = [];
      await expect(
        observeRunlingEvents(
          (event) => failed.push(event),
          () => docker.build(ctx, { contextDirectory: directory })
        )
      ).rejects.toMatchObject({ name: 'DockerOperationError', exitCode: 1 });
      expect(failed.filter((event) => event.type === 'command.finished')).toEqual([
        expect.objectContaining({ status: 'failed' })
      ]);
      for (const cancel of [true, false]) {
        const controller = new AbortController();
        const reason = new Error(cancel ? 'Cancel image build' : 'Build output consumer failed');
        const events: RunlingEvent[] = [];
        await expect(
          observeRunlingEvents(
            (event) => events.push(event),
            () =>
              docker.build(
                { ...ctx, signal: controller.signal },
                {
                  contextDirectory: directory,
                  onOutput: () => {
                    if (cancel) controller.abort(reason);
                    else throw reason;
                  }
                }
              )
          )
        ).rejects.toBe(reason);
        expect(events.filter((event) => event.type === 'command.finished')).toEqual([
          expect.objectContaining({ status: 'failed' })
        ]);
      }
    } finally {
      await rm(directory, { recursive: true, force: true });
    }
  }, 60_000);

  test('streams both outputs before exit, masks split secrets, and removes a successful job', async () => {
    const events: RunlingEvent[] = [];
    const output: string[] = [];
    let notify!: () => void;
    const streamed = new Promise<void>((resolve) => {
      notify = resolve;
    });
    let completed = false;
    const execution = observeRunlingEvents(
      (event) => events.push(event),
      () =>
        docker.run(ctx, {
          image,
          env: { TOKEN: 'private-build-token' },
          command: [
            'sh',
            '-c',
            'printf private-build; sleep 0.1; printf -- "-token\\n"; printf "warning\\n" >&2; sleep 1'
          ],
          onOutput: ({ text }) => {
            output.push(text);
            if (output.join('').includes('[REDACTED]')) notify();
          }
        })
    );
    void execution.then(() => {
      completed = true;
    });
    await streamed;
    expect(completed).toBe(false);
    const result = await execution;
    expect(result.exitCode).toBe(0);
    expect(output.join('')).not.toContain('private-build-token');
    expect(output.join('')).toContain('[REDACTED]');
    expect(output.join('')).toContain('warning');
    expect(JSON.stringify(events)).not.toContain('private-build-token');
    expect(events.filter((event) => event.type === 'command.finished')).toHaveLength(1);
    expect((await execa('docker', ['inspect', result.id], { reject: false })).exitCode).not.toBe(0);
  });

  test('preserves nonzero exit codes and removes failed jobs', async () => {
    await expect(docker.run(ctx, { image, command: ['sh', '-c', 'exit 7'] })).rejects.toMatchObject(
      {
        name: 'DockerExitError',
        exitCode: 7
      }
    );
  });

  test('cancellation waits for daemon-side removal, including concurrent disposal', async () => {
    const controller = new AbortController();
    const reason = new Error('Cancel integration build');
    const container = await docker.start(
      { ...ctx, signal: controller.signal },
      {
        image,
        command: ['sleep', '60']
      }
    );
    controller.abort(reason);
    await expect(container.wait()).rejects.toBe(reason);
    await Promise.all([container.dispose(), container.dispose()]);
    expect((await execa('docker', ['inspect', container.id], { reject: false })).exitCode).not.toBe(
      0
    );
  });

  test('deadlines remove containers even when the process ignores SIGTERM', async () => {
    await expect(
      docker.run(ctx, { image, command: ['sh', '-c', 'trap "" TERM; sleep 60'], timeoutMs: 1000 })
    ).rejects.toBeInstanceOf(DockerTimeoutError);
  }, 10_000);

  test('owns a ready service and a network used by another job', async () => {
    await using network = await docker.network(ctx);
    await using service = await docker.start(ctx, {
      image,
      network: network.name,
      networkAliases: ['web'],
      command: [
        'sh',
        '-c',
        'while true; do printf "HTTP/1.1 200 OK\\r\\nContent-Length: 6\\r\\nConnection: close\\r\\n\\r\\nready\\n" | nc -l -p 8080 >/dev/null; done'
      ],
      ready: { command: ['wget', '-q', '-O', '/dev/null', 'http://127.0.0.1:8080'] }
    });
    const chunks: string[] = [];
    await docker.run(ctx, {
      image,
      network: network.name,
      command: ['wget', '-qO-', 'http://web:8080'],
      onOutput: ({ text }) => chunks.push(text)
    });
    expect(chunks.join('')).toBe('ready\n');
  });

  test('a slow readiness command times out and cleans up the service', async () => {
    const events: RunlingEvent[] = [];
    await expect(
      observeRunlingEvents(
        (event) => events.push(event),
        () =>
          docker.start(ctx, {
            image,
            command: ['sleep', '60'],
            ready: { command: ['sleep', '60'], timeoutMs: 200 }
          })
      )
    ).rejects.toBeInstanceOf(DockerTimeoutError);
    expect(events.filter((event) => event.type === 'command.finished')).toMatchObject([
      { status: 'failed' }
    ]);
  });

  test('normal service disposal completes its activity and applies resource options', async () => {
    const events: RunlingEvent[] = [];
    await observeRunlingEvents(
      (event) => events.push(event),
      async () => {
        await using service = await docker.start(ctx, {
          image,
          command: ['sleep', '60'],
          workdir: '/tmp',
          user: '65534:65534',
          cpus: 0.5,
          memory: '64m',
          readOnly: true
        });
        const { stdout } = await execa('docker', ['inspect', service.id]);
        const [info] = JSON.parse(stdout);
        expect(info.Config.Labels['dev.runling.run']).toBe(runId);
        expect(info.Config.WorkingDir).toBe('/tmp');
        expect(info.Config.User).toBe('65534:65534');
        expect(info.HostConfig.ReadonlyRootfs).toBe(true);
        expect(info.HostConfig.Memory).toBe(64 * 1024 * 1024);
        expect(info.HostConfig.NanoCpus).toBe(500_000_000);
      }
    );
    expect(events.filter((event) => event.type === 'command.finished')).toMatchObject([
      { status: 'completed' }
    ]);
  });

  test('copies inputs and artifacts and mounts paths with spaces', async () => {
    const directory = await mkdtemp(join(tmpdir(), 'runling docker integration '));
    try {
      await writeFile(join(directory, 'input.txt'), 'artifact input');
      await using container = await docker.start(ctx, {
        image,
        command: ['sleep', '60'],
        mounts: [{ source: directory, target: '/input', readOnly: true }]
      });
      await container.copyTo(join(directory, 'input.txt'), '/copied.txt');
      await container.copyFrom('/copied.txt', join(directory, 'output.txt'));
      expect(await readFile(join(directory, 'output.txt'), 'utf8')).toBe('artifact input');
      await container.copyFrom('/input/input.txt', join(directory, 'mounted.txt'));
      expect(await readFile(join(directory, 'mounted.txt'), 'utf8')).toBe('artifact input');
    } finally {
      await rm(directory, { recursive: true, force: true });
    }
  });

  test('a throwing output consumer removes its running container', async () => {
    await expect(
      docker.run(ctx, {
        image,
        command: ['sh', '-c', 'echo started; sleep 60'],
        onOutput: () => {
          throw new Error('Consumer failed');
        }
      })
    ).rejects.toThrow('Consumer failed');
  });

  test('a stopped job retains artifacts until disposal', async () => {
    const directory = await mkdtemp(join(tmpdir(), 'runling-artifact-'));
    try {
      await using container = await docker.start(ctx, {
        image,
        command: ['sh', '-c', 'echo artifact >/result.txt']
      });
      await container.wait();
      await container.copyFrom('/result.txt', join(directory, 'result.txt'));
      expect(await readFile(join(directory, 'result.txt'), 'utf8')).toBe('artifact\n');
    } finally {
      await rm(directory, { recursive: true, force: true });
    }
  });
});
