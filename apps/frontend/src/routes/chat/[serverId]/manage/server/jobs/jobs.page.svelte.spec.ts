import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { Code, ConnectError } from '@connectrpc/connect';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { createTestServerScope } from '$lib/test-utils/serverScope.svelte';
import { queryClient } from '$lib/query/client';
import { JobStatus, RetryJobState, type AdminJob } from '$lib/api/adminJobs';
import JobsPage from './+page.svelte';

const mocks = vi.hoisted(() => ({ listJobs: vi.fn(), retryJob: vi.fn() }));
vi.mock('$lib/render/pageTitle', () => ({ formatPageTitle: () => 'Chatto' }));
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

/** Captures the table sentinel so a test can request the next page. */
let sentinel: IntersectionObserverCallback | undefined;
let originalIntersectionObserver: typeof IntersectionObserver;

beforeAll(() => loadLocaleMessages('en-GB'));
beforeEach(() => {
  queryClient.clear();
  vi.resetAllMocks();
  createTestServerScope({ api: mocks });
  originalIntersectionObserver = globalThis.IntersectionObserver;
  globalThis.IntersectionObserver = class {
    constructor(callback: IntersectionObserverCallback) {
      sentinel = callback;
    }
    observe() {}
    unobserve() {}
    disconnect() {}
    takeRecords() {
      return [];
    }
  } as unknown as typeof IntersectionObserver;
});
afterEach(() => {
  queryClient.clear();
  globalThis.IntersectionObserver = originalIntersectionObserver;
  sentinel = undefined;
});

function job(id: string, changes: Partial<AdminJob> = {}): AdminJob {
  return {
    id,
    type: 'webhook',
    status: JobStatus.FAILED,
    attempts: 3,
    maxAttempts: 5,
    failure: 'http_error',
    retryState: RetryJobState.ALLOWED,
    retryVersion: 'a'.repeat(64),
    createdAt: new Date('2026-11-10T12:00:00Z'),
    updatedAt: null,
    nextAttemptAt: null,
    ...changes
  };
}

describe('job queue list', () => {
  it('shows load errors and refreshes into the empty state', async () => {
    mocks.listJobs
      .mockRejectedValueOnce(new ConnectError('denied', Code.PermissionDenied))
      .mockResolvedValue({ jobs: [], nextPageToken: '' });
    const screen = render(JobsPage);
    await expect.element(screen.getByText('You do not have permission to do that.')).toBeVisible();
    await screen.getByRole('button', { name: 'Refresh' }).click();
    await expect.element(screen.getByText('No jobs yet')).toBeVisible();
  });

  it('renders one linked row per job and loads the next page from the table sentinel', async () => {
    mocks.listJobs
      .mockResolvedValueOnce({ jobs: [job('first')], nextPageToken: 'next' })
      .mockResolvedValueOnce({
        jobs: [
          job('first'),
          job('second', { type: 'asset_cleanup', status: JobStatus.QUEUED, failure: '' })
        ],
        nextPageToken: ''
      });
    const screen = render(JobsPage);
    const link = screen.getByRole('link', { name: 'Webhook delivery' });
    await expect
      .element(link)
      .toHaveAttribute('href', expect.stringMatching(/\/manage\/server\/jobs\/first$/));
    await expect.element(screen.getByRole('cell', { name: 'Failed' })).toBeVisible();
    await expect.element(screen.getByText('3 / 5')).toBeVisible();
    await expect.element(screen.getByText('http_error')).toBeVisible();

    await expect.poll(() => sentinel).toBeDefined();
    sentinel!([{ isIntersecting: true } as IntersectionObserverEntry], {} as IntersectionObserver);
    await expect.element(screen.getByRole('link', { name: 'File cleanup' })).toBeVisible();
    expect(screen.container.querySelectorAll('a.data-table-row-link')).toHaveLength(2);
    expect(mocks.listJobs.mock.calls[1][0]).toMatchObject({ pageToken: 'next' });
  });

  it('retries a failed job from its row and shows activity for running jobs', async () => {
    mocks.listJobs
      .mockResolvedValueOnce({
        jobs: [job('failed'), job('expired', { retryState: RetryJobState.EXPIRED })],
        nextPageToken: ''
      })
      .mockResolvedValue({
        jobs: [
          job('failed', { status: JobStatus.RUNNING, retryState: RetryJobState.NOT_FAILED }),
          job('expired', { retryState: RetryJobState.EXPIRED })
        ],
        nextPageToken: ''
      });
    mocks.retryJob.mockResolvedValue(
      job('failed', { status: JobStatus.QUEUED, retryState: RetryJobState.NOT_FAILED })
    );
    const screen = render(JobsPage);
    const retries = screen.getByRole('button', { name: 'Try Again' });
    await expect.element(retries.nth(0)).toBeEnabled();
    await expect.element(retries.nth(1)).toBeDisabled();
    await retries.nth(0).click();
    expect(mocks.retryJob).toHaveBeenCalledExactlyOnceWith('failed', 'a'.repeat(64));
    await expect.element(screen.getByText('Running', { exact: true })).toBeVisible();
    expect(screen.container.querySelectorAll('[data-activity-indicator]')).toHaveLength(1);
    expect(screen.container.querySelectorAll('button')).toHaveLength(2);
  });

  it('requests the selected status filter', async () => {
    mocks.listJobs.mockResolvedValue({ jobs: [job('failed')], nextPageToken: '' });
    const screen = render(JobsPage);
    await expect.element(screen.getByRole('link', { name: 'Webhook delivery' })).toBeVisible();
    expect(mocks.listJobs.mock.calls[0][0]).toMatchObject({ status: JobStatus.UNSPECIFIED });
    await screen.getByRole('radio', { name: 'Failed' }).click();
    await expect
      .poll(() => mocks.listJobs.mock.calls.some(([input]) => input.status === JobStatus.FAILED))
      .toBe(true);
  });
});
