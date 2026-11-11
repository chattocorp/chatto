import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { Code, ConnectError } from '@connectrpc/connect';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { createTestServerScope } from '$lib/test-utils/serverScope.svelte';
import { queryClient } from '$lib/query/client';
import { removeRegisteredAdminQueries } from '$lib/query/cacheRegistry';
import { adminQueryKeys } from '$lib/query/admin';
import { JobStatus, RetryJobState, type AdminJob } from '$lib/api/adminJobs';
import JobPage from './+page.svelte';

const mocks = vi.hoisted(() => ({ getJob: vi.fn(), retryJob: vi.fn(), id: 'job-1' }));
vi.mock('$lib/render/pageTitle', () => ({ formatPageTitle: () => 'Chatto' }));
vi.mock('$app/state', () => ({
  page: {
    get params() {
      return { id: mocks.id };
    }
  }
}));
vi.mock('$app/paths', () => ({
  base: '',
  assets: '',
  resolve: (path: string, params?: Record<string, string>) =>
    path.replace('[serverId]', params?.serverId ?? '')
}));
vi.mock(
  '$lib/state/server/scope.svelte',
  async () => (await import('$lib/test-utils/serverScope.svelte')).serverScopeModule
);

beforeAll(() => loadLocaleMessages('en-GB'));
beforeEach(() => {
  queryClient.clear();
  vi.resetAllMocks();
  mocks.id = 'job-1';
  createTestServerScope({ api: mocks });
});
afterEach(() => queryClient.clear());

function job(changes: Partial<AdminJob> = {}): AdminJob {
  return {
    id: 'job-1',
    type: 'webhook',
    status: JobStatus.FAILED,
    attempts: 5,
    maxAttempts: 5,
    failure: 'transport_error',
    retryState: RetryJobState.ALLOWED,
    retryVersion: 'a'.repeat(64),
    createdAt: new Date('2026-11-10T12:00:00Z'),
    updatedAt: new Date('2026-11-10T12:10:00Z'),
    nextAttemptAt: null,
    ...changes
  };
}

const queued = { status: JobStatus.QUEUED, retryState: RetryJobState.NOT_FAILED };

describe('job details', () => {
  it('shows safe details and a missing job', async () => {
    mocks.getJob.mockResolvedValueOnce(job());
    const screen = render(JobPage);
    await expect.element(screen.getByRole('heading', { name: 'Webhook delivery' })).toBeVisible();
    await expect.element(screen.getByText('transport_error')).toBeVisible();
    await expect.element(screen.getByText('5 / 5')).toBeVisible();
    await expect.element(screen.getByText('job-1')).toBeVisible();
    screen.unmount();

    mocks.getJob.mockResolvedValueOnce(null);
    const missing = render(JobPage);
    await expect.element(missing.getByText('This job no longer exists.')).toBeVisible();
  });

  it.each([
    [
      'expired',
      { retryState: RetryJobState.EXPIRED },
      'This job has expired and cannot be retried.'
    ],
    ['unknown', { retryState: undefined }, 'This action is currently unavailable.'],
    ['versionless', { retryVersion: '' }, 'This action is currently unavailable.']
  ])('disables retry for a %s retry state', async (_name, changes, hint) => {
    mocks.getJob.mockResolvedValue(job(changes));
    const screen = render(JobPage);
    await expect.element(screen.getByText(hint)).toBeVisible();
    await expect.element(screen.getByRole('button', { name: 'Try Again' })).toBeDisabled();
  });

  it('retries the rendered budget and hides retry while queued', async () => {
    mocks.getJob.mockResolvedValueOnce(job()).mockResolvedValue(job(queued));
    mocks.retryJob.mockResolvedValue(job(queued));
    const screen = render(JobPage);
    await screen.getByRole('button', { name: 'Try Again' }).click();
    await expect.element(screen.getByText('Queued', { exact: true })).toBeVisible();
    await expect
      .poll(() => screen.container.querySelector('button[type="button"]')?.textContent ?? '')
      .not.toContain('Try Again');
    expect(mocks.retryJob).toHaveBeenCalledExactlyOnceWith('job-1', 'a'.repeat(64));
  });

  it('reloads the job after a stale retry and echoes the new version', async () => {
    mocks.getJob
      .mockResolvedValueOnce(job())
      .mockResolvedValueOnce(job({ retryVersion: 'b'.repeat(64), failure: 'new_failure' }))
      .mockResolvedValue(job({ ...queued, retryVersion: 'b'.repeat(64) }));
    mocks.retryJob
      .mockRejectedValueOnce(new ConnectError('changed', Code.Aborted))
      .mockResolvedValue(job(queued));
    const screen = render(JobPage);
    await screen.getByRole('button', { name: 'Try Again' }).click();
    await expect.element(screen.getByText('new_failure')).toBeVisible();
    await expect.element(screen.getByRole('button', { name: 'Try Again' })).toBeEnabled();
    await screen.getByRole('button', { name: 'Try Again' }).click();
    await expect.element(screen.getByText('Queued', { exact: true })).toBeVisible();
    expect(mocks.retryJob).toHaveBeenNthCalledWith(1, 'job-1', 'a'.repeat(64));
    expect(mocks.retryJob).toHaveBeenNthCalledWith(2, 'job-1', 'b'.repeat(64));
  });

  it('does not restore job data when a retry finishes after an authority reset', async () => {
    const scope = createTestServerScope({ api: mocks });
    mocks.getJob.mockResolvedValue(job({ failure: 'private_failure' }));
    let resolveRetry!: (value: AdminJob) => void;
    mocks.retryJob.mockImplementation(
      () =>
        new Promise<AdminJob>((resolve) => {
          resolveRetry = resolve;
        })
    );
    const screen = render(JobPage);
    await screen.getByRole('button', { name: 'Try Again' }).click();
    mocks.getJob.mockRejectedValue(new ConnectError('denied', Code.PermissionDenied));
    removeRegisteredAdminQueries(scope.serverId);
    resolveRetry(job({ ...queued, failure: 'private_failure' }));
    await expect.poll(() => screen.container.textContent?.includes('private_failure')).toBe(false);
    await expect
      .poll(() =>
        queryClient.getQueryData(
          adminQueryKeys.job(scope.serverId, scope.scope.connection, 'job-1')
        )
      )
      .toBeUndefined();
  });
});
