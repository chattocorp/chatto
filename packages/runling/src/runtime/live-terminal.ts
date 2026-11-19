/** CLI-only live status region. Complete task-labelled logs remain in normal scrollback.
 * The renderer owns no input, alternate screen, or journal state. */
import type { Op } from '@bomb.sh/tty';
import { stripVTControlCharacters } from 'node:util';
import type { RunlingEvent } from './events.ts';
import { terminalLogLine } from './terminal-log.ts';
import { formatLogFields } from './log-fields.ts';

/** CI, pipes, JSON, and explicit plain output never initialize a terminal renderer. */
export function supportsLiveTerminal(
  options: { json: boolean; plain?: boolean },
  stdout: { isTTY?: boolean; columns: number; rows: number } = process.stdout,
  stderr: { isTTY?: boolean } = process.stderr,
  env = process.env
): boolean {
  return Boolean(
    !options.json &&
    !options.plain &&
    stdout.isTTY &&
    stderr.isTTY &&
    (!env.CI || env.CI === 'false' || env.CI === '0') &&
    env.TERM !== 'dumb' &&
    env.NO_COLOR === undefined &&
    env.FORCE_COLOR !== '0' &&
    stdout.columns >= 30 &&
    stdout.rows >= 10
  );
}

interface TaskRow {
  id: string;
  label: string;
  started: number;
  status: 'running' | 'completed' | 'failed';
  duration?: number;
}

/** Allocate a bottom status region only after the caller has checked terminal capabilities.
 * Setup failures fall back to the existing logger. Dispose before printing the final answer. */
export async function createLiveTerminal(
  reference: string,
  stream: Pick<
    NodeJS.WriteStream,
    'columns' | 'rows' | 'write' | 'on' | 'off' | 'writableNeedDrain'
  > = process.stdout,
  errorStream: Pick<NodeJS.WriteStream, 'write'> = process.stderr
) {
  const { createTerm, open, close, text, grow, fixed, rgba } = await import('@bomb.sh/tty');
  const height = 6;
  const term = await createTerm({ width: stream.columns, height });
  const tasks = new Map<string, TaskRow>();
  const channels = new Map<string, string>();
  const waits = new Map<string, string>();
  const parents = new Map<string, string | undefined>();
  const clean = (value: string) =>
    stripVTControlCharacters(value).replace(/[\x00-\x1f\x7f-\x9f]/g, ' ');
  const started = performance.now();
  let disposed = false;
  let drawing = true;
  let broken = false;
  let bottom = stream.rows - height;
  let frame = 0;
  let timer: ReturnType<typeof setInterval> | undefined;

  function reserve() {
    bottom = stream.rows - height;
    // Scroll blank space into the normal buffer, then constrain subsequent logs above it.
    stream.write(
      `\x1b[r\x1b[${stream.rows};1H${'\n'.repeat(height)}\x1b[1;${bottom}r\x1b[${bottom};1H`
    );
  }

  function render() {
    if (!drawing || disposed || stream.writableNeedDrain) return;
    try {
      const rows = [...tasks.values()];
      const active = rows.filter((row) => row.status === 'running');
      const failed = rows.filter((row) => row.status === 'failed');
      const waiting = active.filter(isWaiting);
      // Parents can still do work while children run. Do not call them "waiting".
      const leaves = active.filter(
        (row) =>
          !active.some((child) => child.id !== row.id && ancestors(child.id).includes(row.id))
      );
      const priority = [...failed, ...leaves, ...active, ...rows.slice().reverse()];
      // Select important branches together with their ancestors, then show them
      // in creation order as a tree. Priority must not move a child above its parent.
      const selected = new Set<string>();
      for (const row of priority) {
        const branch = [...ancestors(row.id).reverse(), row.id];
        const missing = branch.filter((id) => !selected.has(id));
        if (selected.size + missing.length <= 4) {
          for (const id of branch) selected.add(id);
        }
      }
      const shown: TaskRow[] = [];
      function appendBranch(parent?: string) {
        for (const row of rows) {
          if (selected.has(row.id) && ancestors(row.id)[0] === parent) {
            shown.push(row);
            appendBranch(row.id);
          }
        }
      }
      appendBranch();
      const seconds = Math.floor((performance.now() - started) / 1000);
      const ops: Op[] = [
        open('status', {
          layout: { width: grow(), height: fixed(height), direction: 'ttb' },
          clip: { horizontal: true, vertical: true }
        }),
        open('heading', {
          layout: { width: grow(), height: fixed(1) },
          clip: { horizontal: true, vertical: true }
        }),
        text(` Runling · ${clean(reference)} · ${seconds}s`, { color: rgba(90, 180, 255) }),
        close()
      ];
      for (const row of shown) {
        const symbol =
          row.status === 'running'
            ? isWaiting(row)
              ? '◌'
              : ['◐', '◓', '◑', '◒'][frame % 4]
            : row.status === 'completed'
              ? '✓'
              : '✗';
        const elapsed = Math.floor((row.duration ?? performance.now() - row.started) / 1000);
        ops.push(
          open(`task-${row.id}`, {
            layout: { width: grow(), height: fixed(1) },
            clip: { horizontal: true, vertical: true }
          }),
          text(
            ` ${'  '.repeat(Math.min(ancestors(row.id).length, 3))}${symbol} ${clean(row.label)}${isWaiting(row) ? ' · waiting for input' : ''} · ${elapsed}s`,
            {
              color: row.status === 'failed' ? rgba(255, 100, 100) : rgba(190, 200, 210)
            }
          ),
          close()
        );
      }
      ops.push(
        open('counts', {
          layout: { width: grow(), height: fixed(1) },
          clip: { horizontal: true, vertical: true }
        }),
        text(
          ` ${rows.filter((row) => row.status === 'completed').length} complete · ${active.length - waiting.length} running · ${waiting.length} waiting · ${failed.length} failed${rows.length > shown.length ? ` · +${rows.length - shown.length} hidden` : ''}`
        ),
        close(),
        close()
      );
      const result = term.render(ops, { mode: 'line', row: bottom + 1 });
      if (result.errors.length) throw new Error('Terminal layout failed');
      // Preserve the input cursor, including a partially typed workflow message.
      stream.write(`\x1b7\x1b[${bottom + 1};1H`);
      stream.write(result.output);
      stream.write('\x1b8');
      frame++;
    } catch {
      broken = true;
      stopDrawing();
    }
  }

  function stopDrawing(resized = false) {
    if (!drawing) return;
    drawing = false;
    clearInterval(timer);

    stream.write(
      resized
        ? `\x1b[r\x1b[${stream.rows};1H\n`
        : `\x1b[r\x1b[${Math.max(1, Math.min(bottom + 1, stream.rows))};1H\x1b[J`
    );
  }

  function resize() {
    if (disposed || broken) return;
    // Reflow can move old panel rows into history. Do not erase them: that can
    // also erase log text. Reserve fresh space and redraw at the new dimensions.
    stopDrawing(true);
    if (stream.columns < 30 || stream.rows < 10) return;
    drawing = true;
    stream.write(term.update([{ type: 'resize', width: stream.columns, height }]));
    reserve();
    render();
    if (drawing) startTimer();
  }

  function startTimer() {
    timer = setInterval(render, 100);
    timer.unref();
  }

  /** Return task ancestors through intermediate command activities. */
  function ancestors(id: string): string[] {
    const result: string[] = [];
    const seen = new Set<string>([id]);
    for (let parent = parents.get(id); parent && !seen.has(parent); parent = parents.get(parent)) {
      seen.add(parent);
      if (tasks.has(parent)) result.push(parent);
    }
    return result;
  }

  function isWaiting(row: TaskRow): boolean {
    return (
      row.status === 'running' &&
      [...waits.values()].some((channel) => channels.get(channel) === row.id)
    );
  }

  function taskLabel(id?: string): string {
    for (let current = id; current; current = parents.get(current)) {
      const task = tasks.get(current);
      if (task) return task.label;
    }
    return 'run';
  }

  reserve();
  stream.on('resize', resize);
  render();
  if (drawing) startTimer();
  return {
    /** Project structural events into the status region; print each log event once. */
    event(event: RunlingEvent) {
      if (disposed) return;
      if (event.type === 'task.linked') channels.set(event.channelId, event.taskId);
      else if (event.type === 'wait.started') waits.set(event.id, event.channelId);
      else if (event.type === 'wait.finished') waits.delete(event.id);
      else if (event.type === 'step.started') {
        parents.set(event.id, event.activityId);
        tasks.set(event.id, {
          id: event.id,
          label: event.label,
          started: performance.now(),
          status: 'running'
        });
      } else if (event.type === 'command.started') parents.set(event.id, event.activityId);
      else if (event.type === 'step.finished') {
        const row = tasks.get(event.id);
        if (row) {
          row.status = event.status;
          row.duration = event.durationMs;
          const message = `${row.label} · ${event.status} · ${(event.durationMs / 1000).toFixed(1)}s`;
          stream.write(
            terminalLogLine(event.status === 'failed' ? 'error' : 'success', clean(message)) + '\n'
          );
        }
      } else if (event.type === 'log' && event.source !== 'step') {
        const owner = event.source === 'command' ? event.sourceId : event.activityId;
        const error = event.level === 'error' || event.level === 'warn';
        const destination = error ? errorStream : stream;
        destination.write(
          terminalLogLine(event.level, event.message + formatLogFields(event.fields), {
            context: taskLabel(owner),
            stderr: error
          }) + '\n'
        );
      }
    },
    /** Restore scrolling on success, failure, cancellation, or a second interrupt. */
    dispose() {
      if (disposed) return;
      stopDrawing();
      clearInterval(timer);
      stream.off('resize', resize);
      disposed = true;
      const rows = [...tasks.values()];
      if (!rows.length) return;
      const completed = rows.filter((row) => row.status === 'completed');
      const failed = rows.filter((row) => row.status === 'failed');
      const unfinished = rows.filter((row) => row.status === 'running');
      stream.write(
        terminalLogLine(
          failed.length ? 'error' : unfinished.length ? 'warn' : 'success',
          `${completed.length} complete · ${failed.length} failed · ${unfinished.length} unfinished`
        ) + '\n'
      );
      // Keep the summary compact; the full task history remains in the journal.
      const slowest = completed.sort((a, b) => (b.duration ?? 0) - (a.duration ?? 0)).slice(0, 3);
      for (const row of [...failed, ...unfinished, ...slowest].slice(0, 8)) {
        stream.write(
          terminalLogLine(
            row.status === 'failed' ? 'error' : row.status === 'running' ? 'warn' : 'info',
            `${clean(row.label)} · ${row.status === 'running' ? 'unfinished' : row.status} · ${((row.duration ?? performance.now() - row.started) / 1000).toFixed(1)}s`
          ) + '\n'
        );
      }
    }
  };
}
