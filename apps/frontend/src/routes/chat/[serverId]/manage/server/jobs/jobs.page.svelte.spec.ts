import { beforeAll, beforeEach, afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { Code, ConnectError } from '@connectrpc/connect';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { createTestServerScope } from '$lib/test-utils/serverScope.svelte';
import { queryClient } from '$lib/query/client';
import { removeRegisteredAdminQueries } from '$lib/query/cacheRegistry';
import { adminQueryKeys } from '$lib/query/admin';
import { JobStatus, RetryJobState, type AdminJob } from '$lib/api/adminJobs';
import JobsPage from './+page.svelte';

const mocks = vi.hoisted(() => ({ listJobs: vi.fn(), retryJob: vi.fn() }));
vi.mock('$lib/render/pageTitle', () => ({ formatPageTitle: () => 'Chatto' }));
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

function job(id: string, changes: Partial<AdminJob> = {}): AdminJob {
  return {
    id,
    type: 'webhook',
    status: JobStatus.FAILED,
    attempts: 3,
    maxAttempts: 3,
    failure: 'http_error',
    retryState: RetryJobState.ALLOWED,
    retryVersion: 'a'.repeat(64),
    createdAt: new Date('2026-11-10T12:00:00Z'),
    updatedAt: null,
    nextAttemptAt: null,
    ...changes
  };
}

describe('job queue settings', () => {
  it('keeps load errors in the queue panel and refreshes into the empty state', async () => {
    mocks.listJobs
      .mockRejectedValueOnce(new ConnectError('denied', Code.PermissionDenied))
      .mockResolvedValue({ jobs: [], nextPageToken: '' });
    const screen = render(JobsPage);
    await expect.element(screen.getByText('You do not have permission to do that.')).toBeVisible();
    await screen.getByRole('button', { name: 'Refresh' }).click();
    await expect.element(screen.getByText('No jobs yet')).toBeVisible();
  });

  it('shows safe details, loads another page, and disables expired or unknown retry states', async () => {
    mocks.listJobs
      .mockResolvedValueOnce({
        jobs: [job('expired', { retryState: RetryJobState.EXPIRED })],
        nextPageToken: 'next'
      })
      .mockResolvedValueOnce({
        jobs: [
          job('unknown', { retryState: undefined }),
          job('missing-version', { retryVersion: '' })
        ],
        nextPageToken: ''
      });
    const screen = render(JobsPage);
    await expect.element(screen.getByText('Webhook delivery')).toBeVisible();
    await screen.container.querySelector('summary')!.click();
    await expect.element(screen.getByText('expired', { exact: true })).toBeVisible();
    await expect
      .element(screen.getByText('This job has expired and cannot be retried.'))
      .toBeVisible();
    await expect.element(screen.getByRole('button', { name: 'Try Again' })).toBeDisabled();
    await screen.getByRole('button', { name: 'Load more' }).click();
    await expect.poll(() => screen.container.querySelectorAll('summary').length).toBe(3);
    screen.container.querySelectorAll('summary')[1].click();
    screen.container.querySelectorAll('summary')[2].click();
    const buttons = screen.container.querySelectorAll<HTMLButtonElement>('details button');
    expect(buttons.length).toBe(3);
    expect([...buttons].every((button) => button.disabled)).toBe(true);
    expect(mocks.listJobs.mock.calls[1][0]).toMatchObject({ pageToken: 'next' });
  });

  it('updates a retried job and does not offer another retry while queued', async () => {
    mocks.listJobs
      .mockResolvedValueOnce({ jobs: [job('retry-me')], nextPageToken: '' })
      .mockResolvedValue({
        jobs: [job('retry-me', { status: JobStatus.QUEUED, retryState: RetryJobState.NOT_FAILED })],
        nextPageToken: ''
      });
    mocks.retryJob.mockResolvedValue(
      job('retry-me', { status: JobStatus.QUEUED, retryState: RetryJobState.NOT_FAILED })
    );
    const screen = render(JobsPage);
    await expect.element(screen.getByText('Webhook delivery')).toBeVisible();
    screen.container.querySelector('summary')!.click();
    await screen.getByRole('button', { name: 'Try Again' }).click();
    await expect.element(screen.getByText('Queued', { exact: true })).toBeVisible();
    await expect.poll(() => screen.container.querySelectorAll('details button').length).toBe(0);
    expect(mocks.retryJob).toHaveBeenCalledExactlyOnceWith('retry-me', 'a'.repeat(64));
  });

  it('echoes the rendered retry version and refreshes it after a stale retry is rejected', async () => {
    mocks.listJobs
      .mockResolvedValueOnce({ jobs: [job('stale')], nextPageToken: '' })
      .mockResolvedValueOnce({
        jobs: [job('stale', { retryVersion: 'b'.repeat(64), failure: 'new_failure' })],
        nextPageToken: ''
      })
      .mockResolvedValue({
        jobs: [
          job('stale', {
            retryVersion: 'b'.repeat(64),
            status: JobStatus.QUEUED,
            retryState: RetryJobState.NOT_FAILED
          })
        ],
        nextPageToken: ''
      });
    mocks.retryJob
      .mockRejectedValueOnce(new ConnectError('changed', Code.Aborted))
      .mockResolvedValue(
        job('stale', { status: JobStatus.QUEUED, retryState: RetryJobState.NOT_FAILED })
      );
    const screen = render(JobsPage);
    await expect.element(screen.getByText('Webhook delivery')).toBeVisible();
    screen.container.querySelector('summary')!.click();
    await screen.getByRole('button', { name: 'Try Again' }).click();
    await expect.element(screen.getByText('new_failure')).toBeVisible();
    await expect.element(screen.getByRole('button', { name: 'Try Again' })).toBeEnabled();
    expect(mocks.retryJob).toHaveBeenNthCalledWith(1, 'stale', 'a'.repeat(64));
    await screen.getByRole('button', { name: 'Try Again' }).click();
    await expect.element(screen.getByText('Queued', { exact: true })).toBeVisible();
    expect(mocks.retryJob).toHaveBeenNthCalledWith(2, 'stale', 'b'.repeat(64));
  });

  it('does not restore private job data when a retry finishes after an authority reset', async () => {
    const scope = createTestServerScope({ api: mocks });
    mocks.listJobs.mockResolvedValue({ jobs: [job('private')], nextPageToken: '' });
    let resolveRetry!: (value: AdminJob) => void;
    mocks.retryJob.mockImplementation(
      () =>
        new Promise<AdminJob>((resolve) => {
          resolveRetry = resolve;
        })
    );
    const screen = render(JobsPage);
    await expect.element(screen.getByText('Webhook delivery')).toBeVisible();
    screen.container.querySelector('summary')!.click();
    await screen.getByRole('button', { name: 'Try Again' }).click();
    mocks.listJobs.mockRejectedValue(new ConnectError('denied', Code.PermissionDenied));
    removeRegisteredAdminQueries(scope.serverId);
    resolveRetry(job('private', { status: JobStatus.QUEUED }));
    await expect.poll(() => screen.container.textContent?.includes('private')).toBe(false);
    await expect
      .poll(() =>
        queryClient.getQueryData(adminQueryKeys.jobs(scope.serverId, scope.scope.connection))
      )
      .toBeUndefined();
  });
});
