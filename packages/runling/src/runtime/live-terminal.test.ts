/** Keep CI output inert and exercise real renderer ownership without claiming stdin. */
import { EventEmitter } from 'node:events';
import { stripVTControlCharacters } from 'node:util';
import { expect, test } from 'vitest';
import { createLiveTerminal, supportsLiveTerminal } from './live-terminal.ts';

const tty = { isTTY: true, columns: 80, rows: 24 };

test('live output requires interactive streams and stays off for CI, JSON, and plain output', () => {
  expect(supportsLiveTerminal({ json: false }, tty, tty, {})).toBe(true);
  for (const env of [
    { CI: 'true' },
    { CI: '1' },
    { TERM: 'dumb' },
    { NO_COLOR: '' },
    { FORCE_COLOR: '0' }
  ])
    expect(supportsLiveTerminal({ json: false }, tty, tty, env)).toBe(false);
  expect(supportsLiveTerminal({ json: true }, tty, tty, {})).toBe(false);
  expect(supportsLiveTerminal({ json: false, plain: true }, tty, tty, {})).toBe(false);
  expect(supportsLiveTerminal({ json: false }, { ...tty, isTTY: false }, tty, {})).toBe(false);
  expect(supportsLiveTerminal({ json: false }, tty, { isTTY: false }, {})).toBe(false);
  expect(supportsLiveTerminal({ json: false }, { ...tty, rows: 5 }, tty, {})).toBe(false);
});

test('labels parallel command output once, restores scrolling, and redraws on resize', async () => {
  const chunks: string[] = [];
  const errors: string[] = [];
  const stream = Object.assign(new EventEmitter(), {
    columns: 80,
    rows: 24,
    writableNeedDrain: false,
    write(chunk: string | Uint8Array) {
      chunks.push(typeof chunk === 'string' ? chunk : new TextDecoder().decode(chunk));
      return true;
    }
  });
  const display = await createLiveTerminal(
    'demo',
    stream as NodeJS.WriteStream,
    {
      write(chunk: string | Uint8Array) {
        errors.push(String(chunk));
        return true;
      }
    } as NodeJS.WriteStream
  );
  try {
    display.event({ type: 'step.started', id: 'backend', label: 'Backend', timestamp: 0 });
    display.event({ type: 'step.started', id: 'frontend', label: 'Frontend', timestamp: 0 });
    display.event({
      type: 'command.started',
      id: 'command',
      command: 'test',
      activityId: 'backend',
      timestamp: 0
    });
    display.event({
      type: 'log',
      level: 'info',
      source: 'command',
      sourceId: 'command',
      message: 'build line',
      color: 'blue',
      depth: 1,
      timestamp: 1
    });
    display.event({
      type: 'log',
      level: 'warn',
      activityId: 'frontend',
      message: 'test warning',
      color: 'gold',
      depth: 1,
      timestamp: 1
    });
    expect(stripVTControlCharacters(chunks.join(''))).toContain('[Backend] ● build line');
    expect(chunks.join('').match(/build line/g)).toHaveLength(1);
    expect(stripVTControlCharacters(errors.join(''))).toContain('[Frontend] ! test warning');
    expect(chunks.join('')).not.toContain('\x1b[?1049h');
    stream.rows = 20;
    stream.columns = 60;
    stream.emit('resize');
    expect(stream.listenerCount('resize')).toBe(1);
    expect(chunks.join('')).toContain('\x1b[1;14r');
    stream.rows = 5;
    stream.emit('resize');
    expect(chunks.at(-1)).not.toContain('\x1b[J');
    stream.rows = 24;
    stream.emit('resize');
    expect(chunks.join('')).toContain('\x1b[1;18r');
    display.event({
      type: 'step.finished',
      id: 'backend',
      status: 'completed',
      durationMs: 10,
      timestamp: 10
    });
    expect(stripVTControlCharacters(chunks.join(''))).toContain('Backend · completed');
  } finally {
    display.dispose();
  }
  const length = chunks.length;
  display.dispose();
  expect(chunks).toHaveLength(length);
  expect(stream.listenerCount('resize')).toBe(0);
});

test('shows input waits and prioritizes failures without breaking tree order', async () => {
  const chunks: string[] = [];
  const stream = Object.assign(new EventEmitter(), {
    columns: 120,
    rows: 24,
    writableNeedDrain: false,
    write(chunk: string | Uint8Array) {
      chunks.push(typeof chunk === 'string' ? chunk : new TextDecoder().decode(chunk));
      return true;
    }
  });
  const display = await createLiveTerminal('status test', stream as NodeJS.WriteStream);
  try {
    display.event({ type: 'step.started', id: 'root', label: 'Parent', timestamp: 0 });
    for (let i = 0; i < 5; i++) {
      display.event({
        type: 'step.started',
        id: `child${i}`,
        label: `Child ${i}`,
        activityId: 'root',
        timestamp: 0
      });
    }
    display.event({ type: 'task.linked', channelId: 'inbox', taskId: 'child0', timestamp: 0 });
    display.event({ type: 'wait.started', id: 'wait', channelId: 'inbox', timestamp: 0 });
    display.event({
      type: 'step.finished',
      id: 'child4',
      status: 'failed',
      durationMs: 200,
      timestamp: 1
    });
    chunks.length = 0;
    // Resize forces a complete frame without relying on timer timing.
    stream.emit('resize');
    const frame = stripVTControlCharacters(chunks.join(''));
    expect(frame).toContain('Child 0 · waiting for input');
    expect(frame).toContain('1 waiting · 1 failed');
    expect(frame).toContain('Child 4');
    expect(frame.indexOf('Parent')).toBeLessThan(frame.indexOf('Child 0'));
    expect(frame.indexOf('Child 0')).toBeLessThan(frame.indexOf('Child 4'));
    expect(frame).not.toContain('Child 2');
    display.event({
      type: 'wait.finished',
      id: 'wait',
      status: 'received',
      durationMs: 1,
      timestamp: 2
    });
    chunks.length = 0;
    stream.emit('resize');
    expect(stripVTControlCharacters(chunks.join(''))).toContain('0 waiting');
  } finally {
    display.dispose();
  }
  const output = stripVTControlCharacters(chunks.join(''));
  expect(output).toContain('0 complete · 1 failed · 5 unfinished');
  expect(output).toContain('Child 4 · failed · 0.2s');
  expect(stream.listenerCount('resize')).toBe(0);
});
