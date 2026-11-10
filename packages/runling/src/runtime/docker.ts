/** Optional Docker CLI integration. Operations inherit task cancellation and run identity;
 * callers own resources with `await using`. Docker credentials and policy remain host-owned. */
import { execa } from 'execa';
import { randomUUID } from 'node:crypto';
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import { isAbsolute, join } from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';
import { emitRunlingEvent } from './events.ts';
import { logCommand } from './log.ts';
import { usePrivateTempDirectory } from './private-temp.ts';
import { OutputMask } from './docker-output.ts';
import type { TaskContext } from './context.ts';

/** Task services used by Docker operations. Passing the context binds cancellation and labels. */
export type DockerTaskContext = Pick<TaskContext, 'signal' | 'run'>;

/** Host configuration for one operation. Importing the module does not connect to Docker. */
export interface DockerOptions {
  /** Use this Docker CLI context; otherwise use the CLI's selected context and environment. */
  context?: string;
  /** Application labels. The `dev.runling.*` prefix is reserved. Exclude private data. */
  labels?: Readonly<Record<string, string>>;
}

/** Build a local image. Build arguments are public configuration, not a secret channel.
 * The context directory is sent to the selected daemon; its .dockerignore controls inclusion. */
export interface DockerBuildOptions {
  contextDirectory: string;
  /** Optional Docker CLI context, distinct from the build context directory. */
  context?: string;
  tag?: string;
  buildArgs?: Readonly<Record<string, string>>;
  /** Additional cancellation, combined with task cancellation. */
  signal?: AbortSignal;
  timeoutMs?: number;
  /** Receive build output without retaining a complete buffer. A throwing callback fails the build. */
  onOutput?: (output: DockerOutput) => void;
}

/** A host bind mount. Paths refer to the Docker daemon's host. */
export interface DockerMount {
  source: string;
  target: string;
  readOnly?: boolean;
}

/** A readiness probe executed inside the service container, without a shell. */
export interface DockerReadiness {
  command: readonly string[];
  /** Default: 30 seconds. Includes time spent executing probes. */
  timeoutMs?: number;
  /** Default: 250 milliseconds. */
  intervalMs?: number;
}

/** One container's configuration. Commands are argument arrays, not shell strings. */
export interface DockerContainerOptions extends DockerOptions {
  image: string;
  /** Omit to use the image's default command. Use `['sh', '-c', script]` for shell syntax. */
  command?: readonly string[];
  /** Environment values use a private file and are masked in streamed output.
   * Values cannot contain newlines. Do not place secrets in command arguments. */
  env?: Readonly<Record<string, string>>;
  mounts?: readonly DockerMount[];
  workdir?: string;
  network?: string;
  networkAliases?: readonly string[];
  cpus?: number;
  /** Docker memory limit, for example `256m`. */
  memory?: string;
  user?: string;
  readOnly?: boolean;
  /** Additional cancellation, combined with the task context's signal. */
  signal?: AbortSignal;
  /** Deadline from startup through container exit. Cleanup can extend beyond the deadline. */
  timeoutMs?: number;
  ready?: DockerReadiness;
  /** Receive masked output as it arrives. Output is not accumulated in memory.
   * A throwing callback fails the operation and removes the container. */
  onOutput?: (output: DockerOutput) => void;
}

/** A decoded output chunk. Ordering is preserved separately for stdout and stderr. */
export interface DockerOutput {
  stream: 'stdout' | 'stderr';
  text: string;
}

/** A successful container exit. Nonzero exits throw `DockerExitError`. */
export interface DockerResult {
  id: string;
  exitCode: number;
}

/** A Docker CLI operation failed. Raw command arguments, output, and environment are omitted. */
export class DockerOperationError extends Error {
  override readonly name = 'DockerOperationError';
  constructor(
    operation: string,
    readonly exitCode?: number
  ) {
    super(`Docker ${operation} failed${exitCode === undefined ? '' : ` (exit ${exitCode})`}`);
  }
}

/** The container process exited unsuccessfully. Its output remains in the workflow log. */
export class DockerExitError extends Error {
  override readonly name = 'DockerExitError';
  constructor(
    readonly id: string,
    readonly exitCode: number
  ) {
    super(`Docker container exited with code ${exitCode}`);
  }
}

/** The container did not finish before its configured deadline. */
export class DockerTimeoutError extends Error {
  override readonly name = 'DockerTimeoutError';
}

/** A disposable network. Dispose it after its containers, including on cancellation. */
export interface DockerNetwork extends AsyncDisposable {
  readonly name: string;
  dispose(): Promise<void>;
}

/** A task-owned container. Successful exit retains files until disposal; failure removes it. */
export interface DockerContainer extends AsyncDisposable {
  readonly id: string;
  readonly name: string;
  /** Wait for exit and all output. A nonzero exit throws `DockerExitError`. */
  wait(): Promise<DockerResult>;
  /** Copy a host file or directory into the container. Both paths must be absolute. */
  copyTo(source: string, target: string): Promise<void>;
  /** Copy artifacts before disposal. Both paths must be absolute. */
  copyFrom(source: string, target: string): Promise<void>;
  /** Stop and remove the container, then await log cleanup. Safe to repeat. */
  dispose(): Promise<void>;
}

const PREFIX = 'dev.runling.';
const CONTROL_TIMEOUT_MS = 30_000;

/** Task-scoped Docker operations. Import from `runling/docker`; no client factory is needed. */
export const docker = {
  /** Build and return an immutable image ID. Streams output as a command activity and removes
   * private build metadata before settling. Images and daemon build caches remain caller-owned. */
  async build(ctx: DockerTaskContext, options: DockerBuildOptions): Promise<string> {
    return new DockerOperations(ctx, options).build(options);
  },
  /** Run to completion and remove the container. Inherits task cancellation and run labels.
   * Nonzero exits throw `DockerExitError`; cleanup completes before this promise settles. */
  async run(ctx: DockerTaskContext, options: DockerContainerOptions): Promise<DockerResult> {
    return new DockerOperations(ctx, options).run(options);
  },
  /** Start a container and optionally wait for readiness. Own the result with `await using`.
   * Task cancellation, nonzero exit, and output failures also trigger removal. */
  async start(ctx: DockerTaskContext, options: DockerContainerOptions): Promise<DockerContainer> {
    return new DockerOperations(ctx, options).start(options);
  },
  /** Create a scoped bridge network. Dispose it after all containers that use it.
   * The task supplies cancellation checks and the run label; cleanup remains scope-owned. */
  async network(ctx: DockerTaskContext, options: DockerOptions = {}): Promise<DockerNetwork> {
    return new DockerOperations(ctx, options).network();
  }
};

/** Runs Docker CLI operations without a shell or Runling's command-argument logging. */
class DockerOperations {
  private readonly cli: DockerCLI;
  constructor(
    private readonly ctx: DockerTaskContext,
    private readonly options: DockerOptions
  ) {
    this.cli = new DockerCLI(options.context);
    for (const key of Object.keys(options.labels ?? {})) {
      if (key.startsWith(PREFIX)) throw new TypeError('The dev.runling.* labels are reserved');
    }
  }

  /** Own CLI execution and its image-ID file; applications supply only build policy. */
  async build(options: DockerBuildOptions): Promise<string> {
    absolutePath(options.contextDirectory);
    deadline(options.timeoutMs);
    if (options.tag && (options.tag.startsWith('-') || /[\s\0]/.test(options.tag)))
      throw new TypeError('Invalid Docker image tag');
    for (const [key, value] of Object.entries(options.buildArgs ?? {})) {
      if (
        !/^[A-Za-z_][A-Za-z0-9_]*$/.test(key) ||
        typeof value !== 'string' ||
        /[\0\r\n]/.test(value)
      )
        throw new TypeError('Invalid Docker build argument');
    }
    const lifetime = new Lifetime(
      [this.ctx.signal, options.signal],
      options.timeoutMs,
      'Docker build timed out'
    );
    const name = `runling-build-${randomUUID()}`;
    const started = performance.now();
    const preview = `Docker build${options.tag ? ` ${options.tag}` : ''}`;
    let directory: string | undefined;
    let status: 'completed' | 'failed' = 'failed';
    emitRunlingEvent({ type: 'command.started', id: name, command: preview });
    logCommand(name, preview);
    try {
      lifetime.signal.throwIfAborted();
      directory = await mkdtemp(join(await usePrivateTempDirectory(), 'docker-build-'));
      const output = join(directory, 'image-id');
      const args = ['build', '--progress=plain', '--iidfile', output];
      if (options.tag) args.push('--tag', options.tag);
      for (const [key, value] of Object.entries(options.buildArgs ?? {}))
        args.push('--build-arg', `${key}=${value}`);
      args.push(options.contextDirectory);
      await this.cli.stream(args, name, lifetime.signal, options.onOutput);
      lifetime.signal.throwIfAborted();
      const image = (await readFile(output, 'utf8')).trim();
      lifetime.signal.throwIfAborted();
      if (!/^sha256:[a-f0-9]{64}$/.test(image))
        throw new DockerOperationError('image build result');
      status = 'completed';
      return image;
    } finally {
      lifetime.close();
      try {
        if (directory) await rm(directory, { recursive: true, force: true });
      } catch (error) {
        status = 'failed';
        throw error;
      } finally {
        emitFinished(name, started, status);
      }
    }
  }

  /** Run a disposable container to completion. Always remove it before resolving or rejecting. */
  async run(options: DockerContainerOptions): Promise<DockerResult> {
    await using container = await this.start(options);
    return await container.wait();
  }

  /** Start a managed container and optionally wait for readiness. The caller must dispose it.
   * A nonzero exit, cancellation, failed probe, or output failure also triggers removal. */
  async start(options: DockerContainerOptions): Promise<DockerContainer> {
    // Keep the values used for masking identical to those supplied at creation, even if the
    // caller later mutates its environment object.
    options = { ...options, env: { ...options.env } };
    validateContainer(options);
    const lifetime = new Lifetime([this.ctx.signal, options.signal], options.timeoutMs);
    const name = `runling-${randomUUID()}`;
    const started = performance.now();
    const preview = `Docker ${options.image}`;
    let directory: string | undefined;
    let creationAttempted = false;
    let container: ManagedContainer | undefined;
    emitRunlingEvent({ type: 'command.started', id: name, command: preview });
    logCommand(name, preview);
    try {
      lifetime.signal.throwIfAborted();
      // Pulling can be cancelled safely: it does not create an owned container. Creation and
      // start finish before observing cancellation so cleanup cannot race daemon-side creation.
      const image = await this.cli.execute(['image', 'inspect', options.image], lifetime.signal);
      if (image.exitCode !== 0)
        await this.cli.control(['image', 'pull', options.image], lifetime.signal);
      const args = ['container', 'create', '--pull=never', '--name', name, ...this.labels(name)];
      if (Object.keys(options.env ?? {}).length) {
        directory = await mkdtemp(join(await usePrivateTempDirectory(), 'docker-env-'));
        const file = join(directory, 'environment');
        await writeFile(
          file,
          Object.entries(options.env ?? {})
            .map(([key, value]) => `${key}=${value}\n`)
            .join(''),
          { mode: 0o600 }
        );
        args.push('--env-file', file);
      }
      for (const mount of options.mounts ?? []) {
        args.push(
          '--mount',
          `type=bind,source=${mount.source},target=${mount.target}${mount.readOnly ? ',readonly' : ''}`
        );
      }
      if (options.workdir) args.push('--workdir', options.workdir);
      if (options.network) args.push('--network', options.network);
      for (const alias of options.networkAliases ?? []) args.push('--network-alias', alias);
      if (options.cpus !== undefined) args.push('--cpus', String(options.cpus));
      if (options.memory) args.push('--memory', options.memory);
      if (options.user) args.push('--user', options.user);
      if (options.readOnly) args.push('--read-only');
      args.push(options.image, ...(options.command ?? []));
      lifetime.signal.throwIfAborted();
      creationAttempted = true;
      const id = await this.cli.control(args, AbortSignal.timeout(CONTROL_TIMEOUT_MS));
      // Delete the environment file before handing ownership to the caller. A failed file
      // cleanup must still go through startup's container cleanup path.
      if (directory) {
        await rm(directory, { recursive: true, force: true });
        directory = undefined;
      }
      lifetime.signal.throwIfAborted();
      await this.cli.control(['container', 'start', name], AbortSignal.timeout(CONTROL_TIMEOUT_MS));
      lifetime.signal.throwIfAborted();
      container = new ManagedContainer(this.cli, id, name, lifetime, options, started);
      if (options.ready) await container.waitUntilReady(options.ready);
      return container;
    } catch (error) {
      if (container) {
        await container.fail(error);
      } else {
        lifetime.close();
        try {
          if (creationAttempted) await this.cli.removeContainer(name);
        } finally {
          emitFinished(name, started, 'failed');
        }
      }
      throw error;
    } finally {
      if (directory) await rm(directory, { recursive: true, force: true });
    }
  }

  /** Create a private bridge network. Ownership is explicit; no global networks are pruned. */
  async network(): Promise<DockerNetwork> {
    this.ctx.signal.throwIfAborted();
    const name = `runling-${randomUUID()}`;
    try {
      await this.cli.control(
        ['network', 'create', ...this.labels(name), name],
        AbortSignal.timeout(CONTROL_TIMEOUT_MS)
      );
    } catch (error) {
      // The daemon can create the network even when the CLI fails to receive its ID.
      await this.cli.removeNetwork(name);
      throw error;
    }
    let removed = false;
    let pending: Promise<void> | undefined;
    const dispose = (): Promise<void> => {
      if (removed) return Promise.resolve();
      return (pending ??= this.cli
        .removeNetwork(name)
        .then(() => {
          removed = true;
        })
        .finally(() => {
          pending = undefined;
        }));
    };
    if (this.ctx.signal.aborted) {
      await dispose();
      this.ctx.signal.throwIfAborted();
    }
    return { name, dispose, [Symbol.asyncDispose]: dispose };
  }

  private labels(resource: string): string[] {
    const labels = {
      ...this.options.labels,
      [`${PREFIX}managed`]: 'true',
      [`${PREFIX}resource`]: resource,
      ...(this.ctx.run ? { [`${PREFIX}run`]: this.ctx.run.id } : {})
    };
    return Object.entries(labels).flatMap(([key, value]) => ['--label', `${key}=${value}`]);
  }
}

/** Private process boundary. Docker errors never retain raw subprocess options or output. */
class DockerCLI {
  constructor(private readonly context?: string) {}

  /** Internal CLI boundary. Failures omit diagnostics that can contain credentials. */
  async execute(
    args: string[],
    signal?: AbortSignal
  ): Promise<{ stdout: string; stderr: string; exitCode: number }> {
    signal?.throwIfAborted();
    try {
      const result = await execa('docker', this.arguments(args), {
        cancelSignal: signal,
        reject: false,
        stdin: 'ignore',
        encoding: 'utf8',
        forceKillAfterDelay: 1000
      });
      signal?.throwIfAborted();
      return { stdout: result.stdout, stderr: result.stderr, exitCode: result.exitCode ?? 1 };
    } catch {
      signal?.throwIfAborted();
      throw new DockerOperationError(args.slice(0, 2).join(' '));
    }
  }

  /** Internal checked CLI operation. */
  async control(args: string[], signal?: AbortSignal): Promise<string> {
    const result = await this.execute(args, signal);
    if (result.exitCode !== 0)
      throw new DockerOperationError(args.slice(0, 2).join(' '), result.exitCode);
    return result.stdout.trim();
  }

  /** Internal cleanup uses its own deadlines, even after the workflow was cancelled. */
  async removeContainer(name: string): Promise<void> {
    // A failed stop must not prevent force removal. The final removal result is authoritative.
    await this.execute(
      ['container', 'stop', '--timeout', '2', name],
      AbortSignal.timeout(CONTROL_TIMEOUT_MS)
    ).catch(() => {});
    const result = await this.execute(
      ['container', 'rm', '--force', '--volumes', name],
      AbortSignal.timeout(CONTROL_TIMEOUT_MS)
    );
    if (result.exitCode !== 0 && !/No such container/i.test(result.stderr))
      throw new DockerOperationError('container removal', result.exitCode);
  }

  /** Cleanup also succeeds when a failed create did not leave a network behind. */
  async removeNetwork(name: string): Promise<void> {
    const result = await this.execute(
      ['network', 'rm', name],
      AbortSignal.timeout(CONTROL_TIMEOUT_MS)
    );
    if (result.exitCode !== 0 && !/network .* not found|No such network/i.test(result.stderr))
      throw new DockerOperationError('network removal', result.exitCode);
  }

  /** Internal streaming operation. Buffers only an incomplete secret and a bounded log line. */
  async follow(name: string, options: DockerContainerOptions, signal: AbortSignal): Promise<void> {
    return this.stream(
      ['container', 'logs', '--follow', name],
      name,
      signal,
      options.onOutput,
      Object.values(options.env ?? {})
    );
  }

  /** Shared process boundary for image builds and container output. Always waits for the CLI
   * and both output consumers to close, including cancellation and callback failures. */
  async stream(
    args: string[],
    name: string,
    signal: AbortSignal,
    onOutput?: (output: DockerOutput) => void,
    maskValues: string[] = []
  ): Promise<void> {
    signal.throwIfAborted();
    const process = execa('docker', this.arguments(args), {
      cancelSignal: signal,
      reject: false,
      buffer: false,
      stdin: 'ignore',
      stdout: 'pipe',
      stderr: 'pipe',
      forceKillAfterDelay: 1000
    });
    // Consumers below can fail before the CLI settles; always observe its rejection.
    void process.catch(() => {});
    const consume = async (stream: AsyncIterable<Uint8Array>, kind: DockerOutput['stream']) => {
      const decoder = new TextDecoder();
      let line = '';
      const output = (text: string) => {
        if (!text) return;
        onOutput?.({ stream: kind, text });
        line += text;
        while (line.includes('\n') || line.length >= 4096) {
          const newline = line.indexOf('\n');
          const end = newline >= 0 && newline < 4096 ? newline : 4096;
          logCommand(name, line.slice(0, end).replace(/\r$/, ''));
          line = line.slice(end + (end === newline ? 1 : 0));
        }
      };
      const mask = new OutputMask(maskValues, output);
      for await (const chunk of stream) mask.write(decoder.decode(chunk, { stream: true }));
      mask.write(decoder.decode());
      mask.end();
      if (line) logCommand(name, line);
    };
    const consumers = [consume(process.stdout, 'stdout'), consume(process.stderr, 'stderr')];
    try {
      await Promise.all(consumers);
      const result = await process.catch(() => {
        signal.throwIfAborted();
        throw new DockerOperationError(args.slice(0, args[0] === 'build' ? 1 : 2).join(' '));
      });
      signal.throwIfAborted();
      if (result.exitCode !== 0)
        throw new DockerOperationError(
          args.slice(0, args[0] === 'build' ? 1 : 2).join(' '),
          result.exitCode
        );
    } catch (error) {
      signal.throwIfAborted();
      throw error;
    } finally {
      process.kill('SIGKILL');
      await Promise.allSettled([process, ...consumers]);
    }
  }

  private arguments(args: string[]): string[] {
    return this.context ? ['--context', this.context, ...args] : args;
  }
}

/** A task-owned container. Successful exit retains files until disposal; failure removes it. */
class ManagedContainer implements DockerContainer {
  private cleanup?: Promise<void>;
  private disposing = false;
  private exited = false;
  private readonly finished: Promise<DockerResult>;
  private readonly onAbort = () => {
    void this.release().catch(() => {});
  };

  /** Internal: instances are returned by `docker.start`. */
  constructor(
    private readonly docker: DockerCLI,
    readonly id: string,
    readonly name: string,
    private readonly lifetime: Lifetime,
    options: DockerContainerOptions,
    started: number
  ) {
    lifetime.signal.addEventListener('abort', this.onAbort, { once: true });
    const waiting = docker.control(['container', 'wait', name], lifetime.signal).then((value) => {
      this.exited = true;
      if (!/^\d+$/.test(value)) throw new DockerOperationError('container wait');
      return Number(value);
    });
    const output = docker.follow(name, options, lifetime.signal);
    this.finished = (async () => {
      try {
        const [code] = await Promise.all([waiting, output]);
        lifetime.signal.throwIfAborted();
        if (code !== 0) throw new DockerExitError(id, code);
        emitFinished(name, started, 'completed');
        return { id, exitCode: code };
      } catch (error) {
        lifetime.abort(error);
        let removed = false;
        try {
          await this.release();
          removed = true;
        } finally {
          await Promise.allSettled([waiting, output]);
          emitFinished(name, started, this.disposing && removed ? 'completed' : 'failed');
        }
        throw error;
      } finally {
        lifetime.close();
      }
    })();
    // A service can exit before its owner awaits wait() or disposes it.
    void this.finished.catch(() => {});
  }

  /** Wait for exit and all output. Rejects on nonzero exit, cancellation, or output failure. */
  wait(): Promise<DockerResult> {
    return this.finished;
  }

  /** Copy host files into a running or successfully exited container. */
  async copyTo(source: string, target: string): Promise<void> {
    absolutePath(source);
    absolutePath(target);
    this.lifetime.signal.throwIfAborted();
    await this.docker.control(
      ['container', 'cp', source, `${this.name}:${target}`],
      this.lifetime.signal
    );
  }

  /** Copy artifacts to the host before disposing a successfully exited container. */
  async copyFrom(source: string, target: string): Promise<void> {
    absolutePath(source);
    absolutePath(target);
    this.lifetime.signal.throwIfAborted();
    await this.docker.control(
      ['container', 'cp', `${this.name}:${source}`, target],
      this.lifetime.signal
    );
  }

  /** Internal readiness loop. A failed or slow probe cannot outlive its readiness deadline. */
  async waitUntilReady(ready: DockerReadiness): Promise<void> {
    const probe = new Lifetime(
      [this.lifetime.signal],
      ready.timeoutMs ?? 30_000,
      'Docker readiness timed out'
    );
    try {
      while (true) {
        probe.signal.throwIfAborted();
        if (this.exited) throw new Error('Docker container exited before becoming ready');
        const result = await this.docker.execute(
          ['container', 'exec', this.name, ...ready.command],
          probe.signal
        );
        if (this.exited) throw new Error('Docker container exited before becoming ready');
        if (result.exitCode === 0) return;
        await delay(ready.intervalMs ?? 250, undefined, { signal: probe.signal });
      }
    } catch (error) {
      probe.signal.throwIfAborted();
      throw error;
    } finally {
      probe.close();
    }
  }

  /** Stop and remove the container, then await log and wait-process cleanup. Safe to repeat. */
  async dispose(): Promise<void> {
    this.disposing ||= !this.lifetime.signal.aborted;
    this.lifetime.abort();
    await this.release();
    await this.finished.catch(() => {});
  }

  /** Startup failed after ownership transferred; preserve failed activity on cleanup. */
  async fail(reason: unknown): Promise<void> {
    this.lifetime.abort(reason);
    await this.dispose();
  }

  async [Symbol.asyncDispose](): Promise<void> {
    await this.dispose();
  }

  private release(): Promise<void> {
    this.lifetime.signal.removeEventListener('abort', this.onAbort);
    return (this.cleanup ??= this.docker.removeContainer(this.name));
  }
}

/** Separate cancellation from cleanup. Timers stop when execution finishes or startup fails. */
class Lifetime {
  private readonly controller = new AbortController();
  private timer?: ReturnType<typeof setTimeout>;
  readonly signal: AbortSignal;
  constructor(
    signals: (AbortSignal | undefined)[],
    timeoutMs?: number,
    message = 'Docker container timed out'
  ) {
    this.signal = AbortSignal.any([
      this.controller.signal,
      ...signals.filter((signal): signal is AbortSignal => signal !== undefined)
    ]);
    if (timeoutMs !== undefined)
      this.timer = setTimeout(
        () => this.controller.abort(new DockerTimeoutError(message)),
        timeoutMs
      );
  }
  abort(reason: unknown = new Error('Docker container was disposed')) {
    this.controller.abort(reason);
  }
  close() {
    clearTimeout(this.timer);
  }
}

function emitFinished(id: string, started: number, status: 'completed' | 'failed') {
  emitRunlingEvent({
    type: 'command.finished',
    id,
    status,
    durationMs: performance.now() - started,
    output: { stdout: '', stderr: '' }
  });
}

function absolutePath(path: string) {
  if (!isAbsolute(path) || /[\0\r\n]/.test(path))
    throw new TypeError('Docker paths must be absolute and cannot contain control characters');
}

function deadline(value: number | undefined) {
  if (value !== undefined && (!Number.isFinite(value) || value <= 0 || value > 2_147_483_647))
    throw new TypeError(
      'Docker deadlines and intervals must be positive milliseconds within the timer limit'
    );
}

function validateContainer(options: DockerContainerOptions) {
  if (!options.image || options.image.startsWith('-') || /[\s\0]/.test(options.image))
    throw new TypeError('A Docker image reference is required');
  deadline(options.timeoutMs);
  deadline(options.ready?.timeoutMs);
  deadline(options.ready?.intervalMs);
  if (options.ready && !options.ready.command.length)
    throw new TypeError('A readiness command is required');
  for (const [key, value] of Object.entries(options.env ?? {})) {
    if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(key) || /[\0\r\n]/.test(value))
      throw new TypeError('Invalid Docker environment name or multiline value');
  }
  for (const mount of options.mounts ?? []) {
    absolutePath(mount.source);
    absolutePath(mount.target);
    if (mount.source.includes(',') || mount.target.includes(','))
      throw new TypeError('Docker mount paths cannot contain commas');
  }
  if (options.workdir) absolutePath(options.workdir);
  if (options.cpus !== undefined && (!Number.isFinite(options.cpus) || options.cpus <= 0))
    throw new TypeError('Docker cpus must be positive');
  if (options.memory !== undefined && !/^\d+[bkmg]?$/i.test(options.memory))
    throw new TypeError('Invalid Docker memory limit');
}
