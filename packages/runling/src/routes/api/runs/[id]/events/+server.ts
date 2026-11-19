import { getRunStore } from '$lib/server/run-store.ts';
import { eventStream } from '$lib/server/stream.ts';
import type { RequestHandler } from './$types';
import { consoleRecord } from '$lib/console-record.ts';

export const GET: RequestHandler = async ({ params, request, url }) => {
  const store = await getRunStore();
  const consoleView = url.searchParams.get('view') === 'console';
  const run = await (consoleView ? store.console(params.id) : store.get(params.id));
  if (!run) return Response.json({ error: 'Run not found.' }, { status: 404 });
  return eventStream(request, (send) => {
    let activityTimer: ReturnType<typeof setTimeout> | undefined;
    let timestamp = 0;
    const unsubscribe = store.subscribe((id, record) => {
      if (id !== params.id) return;
      const projected = consoleView ? consoleRecord(record) : record;
      if (projected) send('record', projected);
      else if (record.type === 'event') {
        timestamp = record.event.timestamp;
        activityTimer ??= setTimeout(() => {
          activityTimer = undefined;
          send('activity', { timestamp });
        }, 100);
      }
    });
    // A new snapshot on reconnect includes any events missed while disconnected.
    send('snapshot', run);
    return () => {
      clearTimeout(activityTimer);
      unsubscribe();
    };
  });
};
