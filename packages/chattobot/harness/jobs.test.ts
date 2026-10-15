import { expect, test, vi } from 'vitest';
import { Jobs, type Job, type JobOutcome } from './jobs.ts';

const thread = { roomId: 'room', threadRootId: 'root' };

function jobs() {
  const progress: [string, string[]][] = [];
  const finished: [Job, JobOutcome][] = [];
  const instance = new Jobs({
    progress: (job, updates) => void progress.push([job.id, updates]),
    finished: (job, outcome) => void finished.push([job, outcome])
  });
  return { instance, progress, finished };
}

test('runs a job in the background, batches its progress, and reports its result', async () => {
  const { instance, progress, finished } = jobs();
  const release = Promise.withResolvers<void>();
  const job = instance.start(
    { id: 'a', kind: 'test', label: 'Testing', replyTo: thread },
    async (report) => {
      report('One');
      report('Two');
      await release.promise;
      return 'Done';
    }
  );
  expect(instance.has('a')).toBe(true);
  expect(instance.list()).toEqual([job]);
  await vi.waitFor(() => expect(progress).toEqual([['a', ['One', 'Two']]]), { timeout: 3000 });
  expect(job.latest).toBe('Two');
  release.resolve();
  await vi.waitFor(() => expect(finished).toHaveLength(1));
  expect(finished[0]![1]).toEqual({ result: 'Done' });
  expect(instance.size).toBe(0);
});

test('reports a failed job and refuses a second job with the same ID', async () => {
  const { instance, finished } = jobs();
  const release = Promise.withResolvers<void>();
  instance.start({ id: 'a', kind: 'test', label: 'Testing', replyTo: thread }, async () => {
    await release.promise;
    throw new Error('Broken');
  });
  expect(() =>
    instance.start({ id: 'a', kind: 'test', label: 'Again', replyTo: thread }, async () => '')
  ).toThrow('Job a is already running');
  release.resolve();
  await vi.waitFor(() => expect(finished).toHaveLength(1));
  expect(finished[0]![1]).toEqual({ error: 'Broken' });
});
