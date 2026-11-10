/** Paged and downloadable output over the existing run journal. */
import { resolve } from 'node:path';
import { getRunStore } from '$lib/server/run-store.ts';
import { journalOutput } from '$lib/server/output-journal.ts';
import type { RequestHandler } from './$types';

export const GET: RequestHandler = async ({ params, url, request }) => {
  const store = await getRunStore();
  if (!store.has(params.id)) return Response.json({ error: 'Run not found.' }, { status: 404 });
  const activity = url.searchParams.get('activity') ?? undefined;
  const integer = (key: string) => {
    const text = url.searchParams.get(key);
    if (text === null) return undefined;
    return /^\d+$/.test(text) && Number.isSafeInteger(Number(text)) ? Number(text) : NaN;
  };
  const before = integer('before');
  const after = integer('after');
  const limit = integer('limit');
  if (
    [before, after, limit].some((value) => value !== undefined && !Number.isFinite(value)) ||
    (before !== undefined && after !== undefined) ||
    (limit !== undefined && (limit < 1 || limit > 500)) ||
    (activity?.length ?? 0) > 256
  )
    return Response.json({ error: 'Invalid output cursor or page size.' }, { status: 400 });
  if (url.searchParams.get('download') === '1') {
    const entries = journalOutput(resolve(store.directory, `${params.id}.jsonl`), {
      activity,
      signal: request.signal
    });
    const encoder = new TextEncoder();
    return new Response(
      new ReadableStream<Uint8Array>({
        async pull(controller) {
          try {
            while (true) {
              const next = await entries.next();
              if (next.done) {
                controller.close();
                return;
              }
              if (next.value.matches) {
                controller.enqueue(encoder.encode(next.value.text + (next.value.more ? '' : '\n')));
                return;
              }
            }
          } catch (error) {
            controller.error(error);
          }
        },
        async cancel() {
          await entries.return(undefined);
        }
      }),
      {
        headers: {
          'content-type': 'text/plain; charset=utf-8',
          'content-disposition': `attachment; filename="run-${params.id}.log"`,
          'cache-control': 'no-store'
        }
      }
    );
  }
  const page = await store.output(params.id, {
    activity,
    before,
    after,
    limit,
    signal: request.signal
  });
  return Response.json(page, { headers: { 'cache-control': 'no-store' } });
};
