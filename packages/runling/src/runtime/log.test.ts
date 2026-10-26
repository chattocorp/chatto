import { ansiColor, terminalColors } from './ansi.ts';
import { afterEach, describe, expect, test, vi } from 'vitest';
import { log, logMarkdown, withLogSource } from './log.ts';
import { observeRunlingEvents, type RunlingEvent } from './events.ts';
import { visibleWidth } from '@earendil-works/pi-tui';
import { stripVTControlCharacters } from 'node:util';
import { step } from './step.ts';
import { withExecutionServices } from './execution.ts';

function terminal(destination: 'stdout' | 'stderr', isTTY: boolean, columns = 80) {
  const stream = process[destination];
  vi.spyOn(process, destination, 'get').mockReturnValue(
    new Proxy(stream, {
      get(target, key, receiver) {
        if (key === 'isTTY') return isTTY;
        if (key === 'columns') return columns;
        return Reflect.get(target, key, receiver);
      }
    })
  );
}

describe('log', () => {
  test('global debug includes source and non-verbose workflow logs', () => {
    vi.stubEnv('RUNLING_LOG_LEVEL', 'debug');
    const events: RunlingEvent[] = [];
    log.withDestination('silent', () =>
      observeRunlingEvents(
        (event) => events.push(event),
        () => {
          log.debug('Source detail');
          withExecutionServices({ verbose: false }, () => log.debug('Workflow detail'));
        }
      )
    );
    expect(events.map((event) => event.type === 'log' && event.message)).toEqual([
      'Source detail',
      'Workflow detail'
    ]);
  });

  test('verbose runs enable debug without changing other runs', () => {
    vi.stubEnv('RUNLING_LOG_LEVEL', undefined);
    expect(withExecutionServices({ verbose: true }, () => log.level)).toBe('debug');
    expect(withExecutionServices({ verbose: false }, () => log.level)).toBe('info');
    expect(log.level).toBe('info');
    log.level = 'debug';
    expect(withExecutionServices({ verbose: false }, () => log.level)).toBe('debug');
  });
  test('keeps concurrent structured fields separate and emits warnings once', async () => {
    const events: RunlingEvent[] = [];
    const output = vi.spyOn(console, 'error').mockImplementation(() => {});
    await observeRunlingEvents(
      (event) => events.push(event),
      () =>
        Promise.all([
          log.withFields({ worker: 'one' }, async () => {
            await Promise.resolve();
            log.warn('Retrying', { count: 2, text: 'one\ntwo' });
          }),
          log.withFields({ worker: 'two' }, async () => {
            await Promise.resolve();
            log.warn('Waiting');
          })
        ])
    );
    expect(events).toEqual([
      expect.objectContaining({
        level: 'warn',
        message: 'Retrying',
        fields: { worker: 'one', count: 2, text: 'one\ntwo' }
      }),
      expect.objectContaining({ level: 'warn', message: 'Waiting', fields: { worker: 'two' } })
    ]);
    expect(output).toHaveBeenCalledTimes(2);
    expect(output.mock.calls[0]![0]).toContain('text="one\\ntwo"');
  });
  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllEnvs();
    log.level = 'info';
  });

  test('debug does not print at the default info level', () => {
    const print = vi.spyOn(console, 'log');
    log.level = 'info';

    log.debug('hidden detail');

    expect(print).not.toHaveBeenCalled();
  });

  test.each(['stdout', 'stderr'] as const)(
    'renders Markdown blocks to the width of %s and preserves journal text',
    (destination) => {
      terminal(destination, true, 46);
      vi.stubEnv('NO_COLOR', undefined);
      vi.stubEnv('FORCE_COLOR', undefined);
      const print = vi
        .spyOn(console, destination === 'stderr' ? 'error' : 'log')
        .mockImplementation(() => {});
      const events: RunlingEvent[] = [];
      const markdown =
        '## Issues\n\n1. **A long issue title that needs to wrap in this terminal.**\n2. [Details](https://example.com/issues/123)';

      observeRunlingEvents(
        (event) => events.push(event),
        () =>
          log.withDestination(destination, () =>
            log.indented(() =>
              withLogSource({ type: 'agent', id: 'agent-1' }, () =>
                logMarkdown('[agent-1]', markdown)
              )
            )
          )
      );

      const rendered = print.mock.calls[0]![0] as string;
      const plain = stripVTControlCharacters(rendered);
      expect(plain).toMatch(/^  ● \[agent-1\]\n/);
      expect(plain).not.toContain('##');
      expect(plain).not.toContain('**');
      expect(plain).not.toContain('[Details](');
      expect(rendered.split('\n').every((line) => visibleWidth(line) <= 46)).toBe(true);
      expect(events).toContainEqual(
        expect.objectContaining({
          type: 'log',
          message: `[agent-1] ${markdown}`,
          source: 'agent',
          sourceId: 'agent-1',
          depth: 1
        })
      );
    }
  );

  test('formats terminal Markdown without escapes when colors are disabled', () => {
    terminal('stdout', true);
    vi.stubEnv('NO_COLOR', '1');
    const print = vi.spyOn(console, 'log').mockImplementation(() => {});
    logMarkdown('[agent-1]', '**Answer**');
    expect(print.mock.calls[0]![0]).toBe('● [agent-1]\n  \n  Answer\n  ');
  });

  test('keeps Markdown intact in pipes and records it without printing in silent mode', () => {
    terminal('stdout', false);
    const print = vi.spyOn(console, 'log').mockImplementation(() => {});
    logMarkdown('[agent-1]', '**Answer**');
    expect(print.mock.calls[0]![0]).toBe('● [agent-1] **Answer**');
    print.mockClear();
    const events: RunlingEvent[] = [];
    observeRunlingEvents(
      (event) => events.push(event),
      () => log.withDestination('silent', () => logMarkdown('[agent-1]', '**Answer**'))
    );
    expect(print).not.toHaveBeenCalled();
    expect(events).toContainEqual(expect.objectContaining({ message: '[agent-1] **Answer**' }));
  });

  test('renders an unlabeled update without changing its journal text', () => {
    terminal('stdout', true);
    const print = vi.spyOn(console, 'log').mockImplementation(() => {});
    const events: RunlingEvent[] = [];
    observeRunlingEvents(
      (event) => events.push(event),
      () => logMarkdown('', '**Answer**')
    );
    const plain = stripVTControlCharacters(print.mock.calls[0]![0]);
    expect(plain).toBe('● Answer\n  ');
    expect(events).toContainEqual(expect.objectContaining({ message: '**Answer**' }));
  });

  test('debug prints at the debug level', () => {
    const print = vi.spyOn(console, 'log');
    log.level = 'debug';

    log.debug('visible detail');

    expect(print).toHaveBeenCalledTimes(1);
    expect(print.mock.calls[0]?.[0]).toContain('visible detail');
  });

  test('info, success, and error print at the info level', () => {
    const print = vi.spyOn(console, 'log');
    const printError = vi.spyOn(console, 'error');
    log.level = 'info';

    log.info('an info message');
    log.success('a success message');
    log.error('an error message');

    expect(print).toHaveBeenCalledTimes(2);
    expect(print.mock.calls[0]?.[0]).toContain('an info message');
    expect(print.mock.calls[1]?.[0]).toContain('a success message');
    expect(printError).toHaveBeenCalledTimes(1);
    expect(printError.mock.calls[0]?.[0]).toContain('an error message');
  });

  test('info, success, and error print at the debug level too', () => {
    const print = vi.spyOn(console, 'log');
    log.level = 'debug';

    log.info('an info message');
    log.success('a success message');

    expect(print).toHaveBeenCalledTimes(2);
  });

  test('highlights text in a bold, bright color', () => {
    vi.stubEnv('NO_COLOR', undefined);
    vi.stubEnv('FORCE_COLOR', '1');
    const highlighted = log.highlight('Reading', '#40c057');

    expect(highlighted.startsWith('\x1b[1m')).toBe(true);
    expect(highlighted).toContain(ansiColor('#40c057') ?? '');
    expect(highlighted).toContain('Reading');
    expect(highlighted.endsWith('\x1b[0m')).toBe(true);
  });

  test.each(['NO_COLOR', 'FORCE_COLOR'])('disables workflow colors with %s', (flag) => {
    vi.stubEnv('NO_COLOR', undefined);
    vi.stubEnv('FORCE_COLOR', '1');
    vi.stubEnv(flag, flag === 'NO_COLOR' ? '1' : '0');
    const print = vi.spyOn(console, 'log').mockImplementation(() => {});
    const printError = vi.spyOn(console, 'error').mockImplementation(() => {});
    log.success(log.highlight('ready'));
    log.error('failed');
    expect(print.mock.calls[0]?.[0]).toBe('✓ ready');
    expect(printError.mock.calls[0]?.[0]).toBe('✗ failed');
    expect(log.withColor('red', () => log.colorize('plain'))).toBe('plain');
  });

  test('uses the destination stream for terminal detection', () => {
    vi.stubEnv('NO_COLOR', undefined);
    vi.stubEnv('FORCE_COLOR', undefined);
    expect(terminalColors({ isTTY: false })).toBe(false);
    expect(terminalColors({ isTTY: true })).toBe(true);
  });

  test('can route informational output to stderr', async () => {
    const print = vi.spyOn(console, 'log');
    const printError = vi.spyOn(console, 'error');

    await log.withDestination('stderr', async () => {
      log.info('machine-readable companion log');
      log.success('completed');
    });

    expect(print).not.toHaveBeenCalled();
    expect(printError).toHaveBeenCalledTimes(2);
  });

  test('indents output one level deeper inside a step', () => {
    const print = vi.spyOn(console, 'log');
    const result = step('Running a step', () => {
      log.info('inside the step');
      return 42;
    });

    expect(result).toBe(42);
    expect(print.mock.calls.map((call) => call[0] as string)).toHaveLength(2);
    expect(print.mock.calls[0]?.[0]).toContain('Running a step');
    expect(print.mock.calls[0]?.[0]).not.toMatch(/^\s/);
    expect(print.mock.calls[1]?.[0]).toContain('inside the step');
    expect(print.mock.calls[1]?.[0]).toMatch(/^ {2}/);
  });

  test('restores the indentation level after a step finishes', async () => {
    const print = vi.spyOn(console, 'log');

    await step('outer', async () => {
      log.info('inside outer');
      await step('inner', async () => {
        log.info('inside inner');
      });
      log.info('back inside outer');
    });
    log.info('after the step');

    const lines = print.mock.calls.map((call) => call[0] as string);
    const depthOf = (message: string) => {
      const line = lines.find((line) => line.includes(message));
      expect(line).toBeDefined();
      return line?.match(/^ */)?.[0].length;
    };

    expect(depthOf('inside outer')).toBe(2);
    expect(depthOf('inside inner')).toBe(4);
    expect(depthOf('back inside outer')).toBe(2);
    expect(depthOf('after the step')).toBe(0);
  });

  test('restores the indentation level when a step throws', async () => {
    const print = vi.spyOn(console, 'log');

    await expect(
      step('failing step', async () => {
        throw new Error('boom');
      })
    ).rejects.toThrow('boom');

    log.info('after the failure');
    expect(print.mock.calls.at(-1)?.[0]).not.toMatch(/^\s/);
  });

  test('keeps concurrent steps at their own indentation level', async () => {
    const print = vi.spyOn(console, 'log');
    let releaseA: () => void = () => {};
    const gate = new Promise<void>((resolveGate) => {
      releaseA = resolveGate;
    });

    const a = step('step a', async () => {
      await gate;
      log.info('from step a');
    });
    const b = step('step b', async () => {
      log.info('from step b');
    });

    await b;
    releaseA();
    await a;

    const lines = print.mock.calls.map((call) => call[0] as string);
    const depthOf = (message: string) =>
      lines.find((line) => line.includes(message))?.match(/^ */)?.[0].length;

    expect(depthOf('from step a')).toBe(2);
    expect(depthOf('from step b')).toBe(2);
  });

  test('supports sync and async work without a label', async () => {
    const print = vi.spyOn(console, 'log');

    const sync = log.indented(() => {
      log.info('sync work');
      return 'sync';
    });
    const async = log.indented(async () => {
      log.info('async work');
      return 'async';
    });

    expect(sync).toBe('sync');
    await expect(async).resolves.toBe('async');
    expect(print.mock.calls[0]?.[0]).toMatch(/^ {2}/);
    expect(print.mock.calls[1]?.[0]).toMatch(/^ {2}/);
  });
});
