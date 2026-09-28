import { expect, test } from 'vitest';
import {
  HostCommandError,
  implementationProcess,
  ImplementationCommandError
} from './implementation-process.ts';

test('keeps failing command diagnostics out of the host error message', async () => {
  try {
    await implementationProcess(
      process.execPath,
      ['-e', "console.error('private diagnostic'); process.exit(2)"],
      { cwd: process.cwd(), signal: new AbortController().signal, captureDiagnostics: true }
    );
    throw new Error('Expected failure');
  } catch (error) {
    expect(error).toBeInstanceOf(ImplementationCommandError);
    expect((error as Error).message).not.toContain('private');
    expect((error as ImplementationCommandError).output).toContain('private diagnostic');
  }
});

test('host command failures discard raw diagnostics by default', async () => {
  await expect(
    implementationProcess(
      process.execPath,
      ['-e', "console.error('private diagnostic'); process.exit(2)"],
      {
        cwd: process.cwd(),
        signal: new AbortController().signal
      }
    )
  ).rejects.not.toHaveProperty('output');
});

test('cancels a shell and a child that ignores SIGTERM', async () => {
  const started = Date.now();
  await expect(
    implementationProcess('bash', ['-c', "trap '' TERM; sleep 30 & wait"], {
      cwd: process.cwd(),
      signal: new AbortController().signal,
      timeoutMs: 100
    })
  ).rejects.toThrow('cancelled or timed out');
  expect(Date.now() - started).toBeLessThan(5000);
});

test('removes selected inherited variables from repository commands', async () => {
  const key = 'CHATTO_ALLOWED_USER_ID';
  const before = process.env[key];
  process.env[key] = 'private-test-value';
  try {
    const output = await implementationProcess(
      process.execPath,
      ['-e', `process.stdout.write(process.env.${key} ?? "absent")`],
      {
        cwd: process.cwd(),
        signal: new AbortController().signal,
        unsetEnv: [key]
      }
    );
    expect(output).toBe('absent');
  } finally {
    if (before === undefined) delete process.env[key];
    else process.env[key] = before;
  }
});

test('failures name the program and subcommand, and report a held git lock', async () => {
  const signal = new AbortController().signal;
  const error = await implementationProcess(
    'sh',
    ['-c', 'echo "fatal: Unable to create \'/x/index.lock\': File exists." >&2; exit 128'],
    { cwd: '.', signal }
  ).catch((reason) => reason);
  expect(error).toBeInstanceOf(HostCommandError);
  expect(error.message).toBe('sh failed (exit 128)');
  expect(error.lockHeld).toBe(true);
  const plain = await implementationProcess('sh', ['-c', 'exit 2'], { cwd: '.', signal }).catch(
    (reason) => reason
  );
  expect(plain.lockHeld).toBe(false);
});
