/** Keep output out of activity projections. The unchanged journal remains the full history. */
import type { RunRecord } from './runs.ts';

export function consoleRecord(record: RunRecord): RunRecord | undefined {
  if (record.type === 'started')
    return {
      ...record,
      run: {
        ...record.run,
        events: record.run.events.flatMap((event) => {
          const projected = consoleRecord({ type: 'event', event });
          return projected?.type === 'event' ? [projected.event] : [];
        })
      }
    };
  if (record.type !== 'event') return record;
  if (record.event.type === 'log') return undefined;
  if (record.event.type === 'command.finished')
    return { ...record, event: { ...record.event, output: { stdout: '', stderr: '' } } };
  if (record.event.type === 'agent.action')
    return { ...record, event: { ...record.event, action: record.event.action.slice(-500) } };
  return record;
}
