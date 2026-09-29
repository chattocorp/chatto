/** Run bounded local commands for implementation setup, validation, and publication. */
import { spawn } from 'node:child_process';

/** Injectable process boundary for local Git/checks and GitHub publication. */
export type ImplementationProcess = (
  command: string,
  args: string[],
  options: {
    cwd: string;
    signal: AbortSignal;
    timeoutMs?: number;
    /** Process-local environment overrides, such as a temporary Git index. */
    env?: Record<string, string>;
    /** Remove inherited settings before launching repository setup or checks. */
    unsetEnv?: string[];
    /** Return local patch/validation diagnostics to the worker. Auth/publication output stays private. */
    captureDiagnostics?: boolean;
    /** Keep the end of large standard output instead of failing at the output limit, for logs. */
    keepTail?: boolean;
  }
) => Promise<string>;

/** A command failure with a host-written message that names only the program and its
 * subcommand, such as `git add failed (exit 128)`. The message can reach users; command output
 * never does. */
export class HostCommandError extends Error {
  /** True when git reported that another process holds a lock, such as `index.lock`. */
  lockHeld = false;
}

/** A failure with captured output, which only the worker's repair context receives. */
export class ImplementationCommandError extends HostCommandError {
  constructor(
    message: string,
    readonly output: string
  ) {
    super(message);
  }
}

/** The program and its first plain subcommand, such as `git add`, without values or paths. */
const commandLabel = (command: string, args: string[]) =>
  [command, args.find((arg) => /^[a-z][a-z-]*$/.test(arg))].filter(Boolean).join(' ');

/** Bound output and stop the process group on cancellation, including test children. */
export const implementationProcess: ImplementationProcess = async (command, args, options) => {
  const signal = AbortSignal.any([
    options.signal,
    AbortSignal.timeout(options.timeoutMs ?? 60_000)
  ]);
  signal.throwIfAborted();
  const label = commandLabel(command, args);
  return new Promise<string>((resolve, reject) => {
    const env: NodeJS.ProcessEnv = {
      ...process.env,
      GIT_TERMINAL_PROMPT: '0',
      GH_PROMPT_DISABLED: '1',
      GH_HOST: 'github.com',
      ...options.env
    };
    for (const key of options.unsetEnv ?? []) delete env[key];
    const child = spawn(command, args, {
      cwd: options.cwd,
      detached: process.platform !== 'win32',
      env,
      stdio: ['ignore', 'pipe', 'pipe']
    });
    let output = '';
    let errors = '';
    let failure: Error | undefined;
    let escalation: ReturnType<typeof setTimeout> | undefined;
    const kill = (signal: NodeJS.Signals) => {
      try {
        if (process.platform !== 'win32' && child.pid) process.kill(-child.pid, signal);
        else child.kill(signal);
      } catch {
        /* The process can exit before cancellation arrives. */
      }
    };
    const stop = () => {
      if (escalation) return;
      kill('SIGTERM');
      escalation = setTimeout(() => kill('SIGKILL'), 1000);
    };
    const receive = (data: Buffer, stderr = false) => {
      if (options.keepTail && !stderr) output = (output + data.toString()).slice(-1_000_000);
      else if (output.length + errors.length + data.length > 1_000_000) {
        failure = new HostCommandError(`${label} exceeded the output limit`);
        stop();
      } else if (stderr) errors += data.toString();
      else output += data.toString();
    };
    child.stdout.on('data', receive);
    // Errors can contain credentials. Keep them out of host errors and logs.
    child.stderr.on('data', (data) => receive(data, true));
    signal.addEventListener('abort', stop, { once: true });
    if (signal.aborted) stop();
    child.on('error', () => {
      failure = new HostCommandError(`${label} could not start`);
    });
    child.on('close', (code) => {
      signal.removeEventListener('abort', stop);
      if (escalation) kill('SIGKILL');
      clearTimeout(escalation);
      if (signal.aborted) reject(new HostCommandError(`${label} was cancelled or timed out`));
      else if (failure) reject(failure);
      else if (code !== 0) {
        const message = `${label} failed (exit ${code ?? 'signal'})`;
        const error = options.captureDiagnostics
          ? new ImplementationCommandError(message, `${output}\n${errors}`)
          : new HostCommandError(message);
        error.lockHeld = /\.lock'?: File exists|Unable to create '[^']*\.lock'/.test(errors);
        reject(error);
      } else resolve(output);
    });
  });
};
