/** Exercise startup failure boundaries without requiring a Docker installation. */
import { readFile, stat, writeFile } from 'node:fs/promises';
import { Readable } from 'node:stream';
import { dirname } from 'node:path';
import { beforeEach, expect, test, vi } from 'vitest';
import { docker, DockerOperationError } from './docker.ts';
import { observeRunlingEvents, type RunlingEvent } from './events.ts';
import { createTaskContext } from './context.ts';

const ctx = createTaskContext();

const cli = vi.hoisted(() => ({ execute: vi.fn() }));
vi.mock('execa', () => ({ execa: cli.execute }));

beforeEach(() => {
  cli.execute.mockReset();
  cli.execute.mockResolvedValue({ stdout: '', stderr: '', exitCode: 0 });
});

test('build owns its private ID file and streams output without retaining a finished buffer', async () => {
  const image = `sha256:${'a'.repeat(64)}`;
  let output = '';
  const events: RunlingEvent[] = [];
  cli.execute.mockImplementation((_program, args: string[]) => {
    output = args[args.indexOf('--iidfile') + 1]!;
    const process = (async () => {
      if (globalThis.process.platform !== 'win32')
        expect((await stat(dirname(output))).mode & 0o777).toBe(0o700);
      await writeFile(output, image + '\n');
      return { exitCode: 0 };
    })();
    return Object.assign(process, {
      stdout: Readable.from([Buffer.from('build output\n')]),
      stderr: Readable.from([Buffer.from('build diagnostics\n')]),
      kill: vi.fn()
    });
  });
  expect(
    await observeRunlingEvents(
      (event) => events.push(event),
      () =>
        docker.build(ctx, {
          contextDirectory: '/build context',
          context: 'test-context',
          tag: 'test:local',
          buildArgs: { VERSION: '1.2.3' }
        })
    )
  ).toBe(image);
  expect(cli.execute.mock.calls[0]![1]).toEqual([
    '--context',
    'test-context',
    'build',
    '--progress=plain',
    '--iidfile',
    output,
    '--tag',
    'test:local',
    '--build-arg',
    'VERSION=1.2.3',
    '/build context'
  ]);
  await expect(stat(dirname(output))).rejects.toMatchObject({ code: 'ENOENT' });
  expect(events).toContainEqual(expect.objectContaining({ type: 'log', message: 'build output' }));
  expect(events).toContainEqual(
    expect.objectContaining({ type: 'log', message: 'build diagnostics' })
  );
  expect(events).toContainEqual(
    expect.objectContaining({
      type: 'command.finished',
      status: 'completed',
      output: { stdout: '', stderr: '' }
    })
  );
});

test('failed builds remove their private files and do not attach CLI arguments to the error', async () => {
  let output = '';
  cli.execute.mockImplementation((_program, args: string[]) => {
    output = args[args.indexOf('--iidfile') + 1]!;
    return Object.assign(Promise.resolve({ exitCode: 7 }), {
      stdout: Readable.from([]),
      stderr: Readable.from([]),
      kill: vi.fn()
    });
  });
  await expect(docker.build(ctx, { contextDirectory: '/build context' })).rejects.toEqual(
    new DockerOperationError('build', 7)
  );
  await expect(stat(dirname(output))).rejects.toMatchObject({ code: 'ENOENT' });
});

test('rejects invalid configuration before any Docker command', async () => {
  for (const options of [
    { contextDirectory: 'relative' },
    { contextDirectory: '/build', timeoutMs: 0 },
    { contextDirectory: '/build', tag: '-invalid' },
    { contextDirectory: '/build', buildArgs: { INVALID: 'line\nbreak' } }
  ])
    await expect(docker.build(ctx, options)).rejects.toBeInstanceOf(TypeError);
  for (const options of [
    { image: '-invalid' },
    { image: 'alpine', timeoutMs: Number.NaN },
    { image: 'alpine', timeoutMs: 0 },
    { image: 'alpine', env: { TOKEN: 'line\nbreak' } },
    { image: 'alpine', ready: { command: [] } },
    { image: 'alpine', mounts: [{ source: 'relative', target: '/work' }] }
  ])
    await expect(docker.run(ctx, options)).rejects.toBeInstanceOf(TypeError);
  await expect(
    docker.network(ctx, { labels: { 'dev.runling.managed': 'override' } })
  ).rejects.toThrow('reserved');
  expect(cli.execute).not.toHaveBeenCalled();
});

test('an already cancelled workflow never calls Docker', async () => {
  const reason = new Error('Cancelled before startup');
  await expect(
    docker.build({ ...ctx, signal: AbortSignal.abort(reason) }, { contextDirectory: '/build' })
  ).rejects.toBe(reason);
  await expect(
    docker.run(
      { ...ctx, signal: AbortSignal.abort(reason) },
      { image: 'alpine', signal: new AbortController().signal }
    )
  ).rejects.toBe(reason);
  expect(cli.execute).not.toHaveBeenCalled();
});

test('failed image pulls never create a container and omit raw diagnostics', async () => {
  cli.execute.mockResolvedValue({
    stdout: '',
    stderr: 'private registry credentials',
    exitCode: 1
  });
  await expect(docker.run(ctx, { image: 'missing:image' })).rejects.toEqual(
    new DockerOperationError('image pull', 1)
  );
  expect(cli.execute.mock.calls.map((call) => call[1])).toEqual([
    ['image', 'inspect', 'missing:image'],
    ['image', 'pull', 'missing:image']
  ]);
});

test('failed creation removes partial resources and private environment files', async () => {
  let envFile = '';
  const events: RunlingEvent[] = [];
  cli.execute.mockImplementation(async (_program, args: string[]) => {
    if (args[1] === 'create') {
      envFile = args[args.indexOf('--env-file') + 1]!;
      expect(await readFile(envFile, 'utf8')).toBe('TOKEN=private-token\n');
      expect((await stat(envFile)).mode & 0o777).toBe(0o600);
      expect(args.join(' ')).not.toContain('private-token');
      return { stdout: '', stderr: 'private-token', exitCode: 1 };
    }
    return { stdout: '', stderr: '', exitCode: 0 };
  });
  await expect(
    observeRunlingEvents(
      (event) => events.push(event),
      () =>
        docker.run(ctx, {
          image: 'alpine',
          env: { TOKEN: 'private-token' }
        })
    )
  ).rejects.toEqual(new DockerOperationError('container create', 1));
  await expect(stat(envFile)).rejects.toMatchObject({ code: 'ENOENT' });
  expect(cli.execute.mock.calls.map((call) => call[1][1])).toContain('rm');
  expect(JSON.stringify(events)).not.toContain('private-token');
  expect(events.filter((event) => event.type === 'command.finished')).toHaveLength(1);
});

test('cancellation during creation waits for creation, then removes the owned name', async () => {
  const controller = new AbortController();
  const reason = new Error('Cancel during create');
  cli.execute.mockImplementation(
    async (_program, args: string[], options: { cancelSignal?: AbortSignal }) => {
      if (args[1] === 'create') {
        controller.abort(reason);
        expect(options.cancelSignal?.aborted).toBe(false);
      }
      if (args[1] === 'rm') expect(options.cancelSignal?.aborted).toBe(false);
      return { stdout: 'container-id', stderr: '', exitCode: 0 };
    }
  );
  await expect(
    docker.start({ ...ctx, signal: controller.signal }, { image: 'alpine' })
  ).rejects.toBe(reason);
  expect(cli.execute.mock.calls.map((call) => call[1][1])).toEqual([
    'inspect',
    'create',
    'stop',
    'rm'
  ]);
});

test('explicit Docker contexts apply to startup and cleanup', async () => {
  cli.execute.mockResolvedValue({ stdout: '', stderr: '', exitCode: 1 });
  await expect(
    docker.start(ctx, { image: 'alpine', context: 'test-context' })
  ).rejects.toBeInstanceOf(DockerOperationError);
  for (const call of cli.execute.mock.calls)
    expect(call[1].slice(0, 2)).toEqual(['--context', 'test-context']);
});
