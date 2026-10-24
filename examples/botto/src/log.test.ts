/** Operational logs describe work without copying private results or external errors. */
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { log, logged } from './log.ts';

function outputMode(stdoutTTY: boolean, stderrTTY = stdoutTTY, colors = false) {
  vi.stubGlobal('process', {
    ...process,
    stdout: { isTTY: stdoutTTY, hasColors: () => colors },
    stderr: { isTTY: stderrTTY, hasColors: () => colors }
  });
}

beforeEach(() => {
  outputMode(true);
  vi.stubEnv('NO_COLOR', undefined);
  vi.stubEnv('TERM', 'xterm-256color');
});

afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

test.each(['info', 'warn', 'error'] as const)(
  'writes structured %s logs to redirected output',
  (level) => {
    outputMode(false);
    const output = vi.spyOn(console, level).mockImplementation(() => {});
    log(level, 'Message accepted', { threadId: 'thread\n"suffix', turn: 2 });
    const line = output.mock.calls[0]![0] as string;
    expect(JSON.parse(line)).toEqual({
      timestamp: expect.any(String),
      level,
      message: 'Message accepted',
      threadId: 'thread\n"suffix',
      turn: 2
    });
    expect(line).not.toContain('\n');
    expect(line).not.toContain('\u001b');
  }
);

test.each([
  ['info', 36],
  ['warn', 33],
  ['error', 31]
] as const)('colors terminal %s severity', (level, code) => {
  outputMode(true, true, true);
  const output = vi.spyOn(console, level).mockImplementation(() => {});
  log(level, 'Message accepted', { threadId: 'thread-123' });
  expect(output.mock.calls[0]![0]).toContain(`\u001b[${code}m${level.toUpperCase()}\u001b[0m`);
  expect(output.mock.calls[0]![0]).toContain('\u001b[2mthreadId="thread-123"\u001b[0m');
});

test.each([
  ['NO_COLOR', '1'],
  ['TERM', 'dumb']
])('respects %s', (key, value) => {
  outputMode(true, true, true);
  vi.stubEnv(key, value);
  const output = vi.spyOn(console, 'info').mockImplementation(() => {});
  log('info', 'Ready');
  expect(output.mock.calls[0]![0]).toMatch(/^\S+ INFO Ready$/);
});

test('detects stdout and stderr independently', () => {
  outputMode(false, true, true);
  const info = vi.spyOn(console, 'info').mockImplementation(() => {});
  const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
  log('info', 'Ready');
  log('warn', 'Stopping');
  expect(JSON.parse(info.mock.calls[0]![0]).level).toBe('info');
  expect(warn.mock.calls[0]![0]).toContain('\u001b[33mWARN');
});

test('logs turn correlation and elapsed time without logging the result', async () => {
  const output = vi.spyOn(console, 'info').mockImplementation(() => {});
  vi.spyOn(performance, 'now').mockReturnValueOnce(100).mockReturnValueOnce(350);
  const result = await logged('Model turn', async () => 'private reply', {
    threadId: 'thread-123',
    turn: 2,
    messages: 3
  });
  expect(result).toBe('private reply');
  expect(output.mock.calls).toEqual([
    [
      expect.stringMatching(
        /^\S+ INFO Model turn completed threadId="thread-123" turn=2 messages=3 durationMs=250$/
      )
    ]
  ]);
});

test('escapes thread IDs so they cannot create extra log lines', () => {
  const output = vi.spyOn(console, 'info').mockImplementation(() => {});
  log('info', 'Message accepted', { threadId: 'thread\n"suffix' });
  expect(output.mock.calls[0]![0]).toContain('threadId="thread\\n\\"suffix"');
  expect(output.mock.calls[0]![0]).not.toContain('\n');
});

test('reports a safe failure hint and rethrows without logging external content', async () => {
  const info = vi.spyOn(console, 'info').mockImplementation(() => {});
  const output = vi.spyOn(console, 'error').mockImplementation(() => {});
  const failure = Object.assign(new Error('private token and message'), { code: 'ECONNREFUSED' });
  await expect(
    logged(
      'Reply post',
      async () => {
        throw failure;
      },
      { turn: 4 }
    )
  ).rejects.toBe(failure);
  expect(info).not.toHaveBeenCalled();
  expect(output).toHaveBeenCalledExactlyOnceWith(
    expect.stringMatching(
      /^\S+ ERROR Reply post failed\. Connection refused\. turn=4 durationMs=\d+$/
    )
  );
});
