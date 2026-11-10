/** Read output from existing JSONL journals with bounded page memory. Activity ancestry is
 * resolved in journal order; selecting a task includes its descendants without copying logs. */
import { stat } from 'node:fs/promises';
import { readRunJournal } from '../../runtime/run-journal.ts';
import { formatLogFields } from '../../runtime/log-fields.ts';
import {
  OUTPUT_PAGE_BYTES,
  OUTPUT_PAGE_SIZE,
  type OutputEntry,
  type OutputPage
} from '../log-page.ts';

export interface OutputQuery {
  activity?: string;
  before?: number;
  after?: number;
  limit?: number;
  signal?: AbortSignal;
}

/** Selection is internal; unselected entries still advance the global cursor. */
interface JournalOutputEntry extends OutputEntry {
  matches: boolean;
}

/** Stream a stable prefix of the journal. Entry boundaries are independent of paging and
 * activity filters. Large buffered output in older journals is split into bounded entries. */
export async function* journalOutput(
  path: string,
  query: OutputQuery = {}
): AsyncGenerator<JournalOutputEntry> {
  const { size } = await stat(path);
  // Agent actions precede their formatted log events. Detect those invocation instances
  // without buffering their output, so old action-only journals remain readable once.
  const formattedAgents = new Set<number>();
  const agentInstances = new Map<string, number>();
  let instanceIndex = 0;
  for await (const { record } of readRunJournal(path, { end: size, signal: query.signal })) {
    instanceIndex++;
    if (record?.type === 'started') agentInstances.clear();
    if (record?.type !== 'event') continue;
    if (record.event.type === 'agent.started')
      agentInstances.set(record.event.agentId, instanceIndex);
    if (record.event.type === 'log' && record.event.source === 'agent') {
      const instance = agentInstances.get(record.event.sourceId ?? '');
      if (instance !== undefined) formattedAgents.add(instance);
    }
  }
  const parents = new Map<string, string | undefined>();
  const agents = new Map<string, string>();
  const steps = new Map<string, { label: string; parent?: string }>();
  let metadataIndex = 0;
  let cursor = 0;
  let origin: number | undefined;
  let offset = 0;
  let lastTimestamp = 0;
  let started = false;
  let recordIndex = 0;
  const belongs = (owner?: string): boolean => {
    if (!query.activity) return true;
    for (let id = owner; id; id = parents.get(id)) if (id === query.activity) return true;
    return false;
  };
  for await (const { record } of readRunJournal(path, { end: size, signal: query.signal })) {
    recordIndex++;
    query.signal?.throwIfAborted();
    if (!record) continue;
    if (record.type === 'started') {
      if (started) metadataIndex++; // The console inserts execution.finished between attempts.
      started = true;
      origin ??= record.run.startedAt;
      offset = record.run.startedAt - origin;
      agents.clear();
      agentInstances.clear();
      continue;
    }
    if (record.type !== 'event') continue;
    const event = record.event;
    lastTimestamp = event.timestamp + offset;
    let owner = event.activityId;
    let text: string | undefined;
    if (
      event.type === 'step.started' ||
      event.type === 'command.started' ||
      event.type === 'wait.started'
    ) {
      parents.set(event.id, event.activityId);
      owner = event.id;
      if (event.type === 'step.started') {
        steps.set(event.id, { label: event.label, parent: event.activityId });
        text = `Started task-${steps.size}`;
      }
    }
    if (event.type === 'agent.started') {
      const id = `${event.agentId}:${metadataIndex}`;
      agents.set(event.agentId, id);
      parents.set(id, event.activityId);
      agentInstances.set(event.agentId, recordIndex);
    }
    if (event.type === 'log') {
      owner =
        event.source === 'agent'
          ? agents.get(event.sourceId ?? '')
          : (event.sourceId ?? event.activityId);
      // Task label logs are emitted in the parent's scope, after the child starts.
      if (event.source === 'step') {
        const step = [...steps].findLast(
          ([, step]) => step.parent === event.activityId && step.label === event.message
        );
        if (step) owner = step[0];
      }
      text = event.message + formatLogFields(event.fields);
    }
    if (event.type === 'command.finished') {
      owner = event.id;
      text = [event.output.stdout, event.output.stderr].filter(Boolean).join('\n');
    }
    if (event.type === 'step.finished') {
      owner = event.id;
      text = `${event.id} ${event.status}${event.error ? ` · ${event.error}` : ''}`;
    }
    if (event.type === 'message.sent')
      text = `Message ${event.direction === 'input' ? 'to' : 'from'} workflow · ${event.payload}`;
    if (event.type === 'agent.action') {
      owner = agents.get(event.agentId);
      text = event.action;
    }
    const hiddenAction =
      event.type === 'agent.action' && formattedAgents.has(agentInstances.get(event.agentId) ?? -1);
    if (event.type !== 'log') metadataIndex++;
    if (text === undefined || (event.type === 'command.finished' && !text)) continue;
    let start = 0;
    do {
      let end = Math.min(text.length, start + 4096);
      if (end < text.length && /[\uD800-\uDBFF]/.test(text[end - 1]!)) end--;
      const entry: JournalOutputEntry = {
        cursor: ++cursor,
        timestamp: lastTimestamp,
        activityId: owner,
        text: text.slice(start, end),
        more: end < text.length,
        matches: !hiddenAction && belongs(owner)
      };
      start = end;
      if (query.before !== undefined && entry.cursor >= query.before) return;
      if (entry.cursor <= (query.after ?? 0)) continue;
      // Advance even for other activities so polling a quiet task does not stall its cursor.
      yield entry;
    } while (start < text.length);
  }
}

/** Return one fixed-size window. Earlier/later navigation replaces the window rather than
 * growing an in-memory history. Reads stop at the journal size captured when they start. */
export async function readOutputPage(path: string, query: OutputQuery = {}): Promise<OutputPage> {
  const limit = Math.max(1, Math.min(500, query.limit ?? OUTPUT_PAGE_SIZE));
  const entries: OutputEntry[] = [];
  let bytes = 0;
  let next = query.after ?? 0;
  let dropped = false;
  let hasLater = false;
  const forward = query.after !== undefined;
  for await (const entry of journalOutput(path, query)) {
    next = entry.cursor;
    if (!entry.matches) continue;
    const length = Buffer.byteLength(entry.text);
    if (
      forward &&
      entries.length &&
      (entries.length >= limit || bytes + length > OUTPUT_PAGE_BYTES)
    ) {
      next = entries.at(-1)!.cursor;
      hasLater = true;
      break;
    }
    const { matches: _, ...selected } = entry;
    entries.push(selected);
    bytes += length;
    while (entries.length > limit || bytes > OUTPUT_PAGE_BYTES) {
      bytes -= Buffer.byteLength(entries.shift()!.text);
      dropped = true;
    }
  }
  return {
    entries,
    next,
    hasEarlier: dropped || (query.after ?? 0) > 0,
    hasLater: hasLater || query.before !== undefined
  };
}
