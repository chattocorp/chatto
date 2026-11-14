import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { createTestServerScope } from '$lib/test-utils/serverScope.svelte';
import { queryClient } from '$lib/query/client';
import { JobStatus, RetryJobState, type AdminJob } from '$lib/api/adminJobs';
import JobsPage from './+page.svelte';
import JobPage from './[id]/+page.svelte';

const mocks = vi.hoisted(() => ({ listJobs: vi.fn(), getJob: vi.fn() }));
// A short interval keeps polling tests fast; production uses the shared constant.
vi.mock('./jobPresentation', async (importOriginal) => ({
  ...(await importOriginal<typeof import('./jobPresentation')>()),
  JOBS_POLL_INTERVAL_MS: 50
}));
vi.mock('$lib/render/pageTitle', () => ({ formatPageTitle: () => 'Chatto' }));
vi.mock('$app/state', () => ({ page: { params: { id: 'job-1' } } }));
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
    retryState: RetryJobState.NOT_FAILED,
    retryVersion: 'a'.repeat(64),
    createdAt: new Date('2026-11-10T12:00:00Z'),
    updatedAt: null,
    nextAttemptAt: null
  };
}

describe('job page polling', () => {
  it('refreshes the list without user action', async () => {
    mocks.listJobs
      .mockResolvedValueOnce({ jobs: [job(JobStatus.QUEUED)], nextPageToken: '' })
      .mockResolvedValue({ jobs: [job(JobStatus.SUCCEEDED)], nextPageToken: '' });
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
});
