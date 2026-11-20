import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { RealtimeProjectionUpdate, type ProjectionHandler } from '@chatto/client/realtime/eventBus';
import { RealtimeEvent } from '@chatto/api-types/realtime/v1/realtime_pb';
import { render } from 'vitest-browser-svelte';
import { Code, ConnectError } from '@connectrpc/connect';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { createTestServerScope } from '$lib/test-utils/serverScope.svelte';
import { queryClient } from '$lib/query/client';
import { JobRetryAvailability, JobStatus, type AdminJob } from '$lib/api/adminJobs';
import JobsPage from './+page.svelte';
import JobPage from './[id]/+page.svelte';

const mocks = vi.hoisted(() => ({ listJobs: vi.fn(), getJob: vi.fn() }));
// A short interval keeps polling tests fast; production uses the shared constant.
vi.mock('./jobPresentation', async (importOriginal) => ({
  ...(await importOriginal<typeof import('./jobPresentation')>()),
  JOBS_POLL_INTERVAL_MS: 50
}));
vi.mock('$lib/render/pageTitle', () => ({ formatPageTitle: () => 'Chatto' }));
vi.mock('$app/state', () => ({
  page: {
    params: { id: 'job-1' },
    url: new URL('https://chat.example.test/chat/-/manage/server/jobs')
  }
}));
vi.mock('$app/navigation', () => ({
  goto: vi.fn(),
  pushState: vi.fn(),
  replaceState: vi.fn(),
  preloadData: vi.fn(),
  invalidate: vi.fn(),
  invalidateAll: vi.fn()
}));
vi.mock('$app/paths', () => ({
  base: '',
  assets: '',
  resolve: (path: string, params?: Record<string, string>) =>
    path.replace('[serverId]', params?.serverId ?? '').replace('[id]', params?.id ?? '')
}));
vi.mock(
  '$lib/state/server/scope.svelte',
  async () => (await import('$lib/test-utils/serverScope.svelte')).serverScopeModule
);

beforeAll(() => loadLocaleMessages('en-GB'));
beforeEach(() => {
  queryClient.clear();
  vi.resetAllMocks();
  createTestServerScope({ api: mocks });
});
afterEach(() => queryClient.clear());

function job(status: JobStatus): AdminJob {
  return {
    id: 'job-1',
    type: 'webhook',
    status,
    attempts: 1,
    maxAttempts: 5,
    failure: '',
    retry: JobRetryAvailability.NOT_FAILED,
    version: 'a'.repeat(64),
    createdAt: new Date('2026-11-10T12:00:00Z'),
    updatedAt: null,
    nextAttemptAt: null
  };
}

describe('job page polling', () => {
  const NETWORK_ERROR = 'Network error. Please try again.';

  it('refreshes on a job signal and on catch-up, with no timer reads', async () => {
    const sync = $state({ watchingJobs: true, lastCaughtUpAt: 1 });
    const onUpdate = vi.fn<(listener: ProjectionHandler) => () => void>(() => () => {});
    createTestServerScope({ api: mocks, store: { realtimeSync: sync, onUpdate } });
    mocks.listJobs
      .mockResolvedValueOnce({ jobs: [job(JobStatus.QUEUED)], nextCursor: '' })
      .mockResolvedValueOnce({ jobs: [job(JobStatus.RUNNING)], nextCursor: '' })
      .mockResolvedValue({ jobs: [job(JobStatus.SUCCEEDED)], nextCursor: '' });
    const screen = render(JobsPage);
    await expect.element(screen.getByRole('cell', { name: 'Queued' })).toBeVisible();
    await new Promise((resolve) => setTimeout(resolve, 200));
    expect(mocks.listJobs).toHaveBeenCalledTimes(1);
    const event = new RealtimeEvent({ event: { case: 'jobsChanged', value: {} } });
    onUpdate.mock.calls[0][0](new RealtimeProjectionUpdate({ event }));
    await expect.element(screen.getByRole('cell', { name: 'Running' })).toBeVisible();
    sync.lastCaughtUpAt++;
    await expect.element(screen.getByRole('cell', { name: 'Succeeded' })).toBeVisible();
  });

  it('keeps a change received during the initial list read', async () => {
    const onUpdate = vi.fn<(listener: ProjectionHandler) => () => void>(() => () => {});
    const realtimeSync = { watchingJobs: true, lastCaughtUpAt: 1 };
    createTestServerScope({ api: mocks, store: { realtimeSync, onUpdate } });
    const initial = Promise.withResolvers<{ jobs: AdminJob[]; nextCursor: string }>();
    mocks.listJobs
      .mockReturnValueOnce(initial.promise)
      .mockResolvedValue({ jobs: [job(JobStatus.SUCCEEDED)], nextCursor: '' });
    const screen = render(JobsPage);
    await vi.waitFor(() => expect(mocks.listJobs).toHaveBeenCalledOnce());
    const event = new RealtimeEvent({ event: { case: 'jobsChanged', value: {} } });
    onUpdate.mock.calls[0][0](new RealtimeProjectionUpdate({ event }));
    initial.resolve({ jobs: [], nextCursor: '' });
    await expect.element(screen.getByRole('cell', { name: 'Succeeded' })).toBeVisible();
    expect(mocks.listJobs).toHaveBeenCalledTimes(2);
  });

  it('polls the list when the connection has no job watch', async () => {
    mocks.listJobs
      .mockResolvedValueOnce({ jobs: [job(JobStatus.QUEUED)], nextCursor: '' })
      .mockResolvedValue({ jobs: [job(JobStatus.SUCCEEDED)], nextCursor: '' });
    const screen = render(JobsPage);
    await expect.element(screen.getByText('Succeeded', { exact: true })).toBeVisible();
  });

  it('refreshes job details until the job reaches a terminal state', async () => {
    mocks.getJob
      .mockResolvedValueOnce(job(JobStatus.QUEUED))
      .mockResolvedValueOnce(job(JobStatus.RUNNING))
      .mockResolvedValue(job(JobStatus.SUCCEEDED));
    const screen = render(JobPage);
    await expect.element(screen.getByText('Succeeded', { exact: true })).toBeVisible();
    const calls = mocks.getJob.mock.calls.length;
    await new Promise((resolve) => setTimeout(resolve, 200));
    expect(mocks.getJob).toHaveBeenCalledTimes(calls);
  });

  it('keeps the loaded rows and shows the error when a list refresh fails', async () => {
    mocks.listJobs
      .mockResolvedValueOnce({ jobs: [job(JobStatus.QUEUED)], nextCursor: '' })
      .mockRejectedValue(new ConnectError('down', Code.Unavailable));
    const screen = render(JobsPage);
    await expect.element(screen.getByText(NETWORK_ERROR)).toBeVisible();
    await expect.element(screen.getByRole('cell', { name: 'Queued' })).toBeVisible();
  });

  it('keeps the loaded job and shows the error when a detail refresh fails', async () => {
    mocks.getJob
      .mockResolvedValueOnce(job(JobStatus.QUEUED))
      .mockRejectedValue(new ConnectError('down', Code.Unavailable));
    const screen = render(JobPage);
    await expect.element(screen.getByText(NETWORK_ERROR)).toBeVisible();
    await expect.element(screen.getByText('Queued', { exact: true })).toBeVisible();
  });

  it('loads the job after a failed first read without a reload', async () => {
    mocks.getJob
      .mockRejectedValueOnce(new ConnectError('down', Code.Unavailable))
      .mockRejectedValueOnce(new ConnectError('down', Code.Unavailable))
      .mockResolvedValue(job(JobStatus.SUCCEEDED));
    const screen = render(JobPage);
    await expect.element(screen.getByText('Succeeded', { exact: true })).toBeVisible();
  });
});
