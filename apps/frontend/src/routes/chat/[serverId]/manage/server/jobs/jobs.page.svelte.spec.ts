import '../../../../../../app.css';
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { Code, ConnectError } from '@connectrpc/connect';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { createTestServerScope } from '$lib/test-utils/serverScope.svelte';
import { queryClient } from '$lib/query/client';
import { JobRetryAvailability, JobStatus, type AdminJob } from '$lib/api/adminJobs';
import { toast } from '$lib/ui/toast';
import JobsPage from './+page.svelte';

const JOBS_URL = 'https://chat.example.test/chat/-/manage/server/jobs';
const mocks = vi.hoisted(() => ({
  listJobs: vi.fn(),
  retryJob: vi.fn(),
  deleteJob: vi.fn(),
  goto: vi.fn()
}));
let currentUrl = $state(new URL(JOBS_URL));
vi.mock('$lib/render/pageTitle', () => ({ formatPageTitle: () => 'Chatto' }));
vi.mock('$lib/ui/toast', () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock('$app/state', () => ({
  page: {
    get url() {
      return currentUrl;
    }
  }
}));
vi.mock('$app/navigation', () => ({
  goto: mocks.goto,
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

/** Captures the table sentinel so a test can request the next page. */
let sentinel: IntersectionObserverCallback | undefined;
let originalIntersectionObserver: typeof IntersectionObserver;

beforeAll(() => loadLocaleMessages('en-GB'));
beforeEach(() => {
  queryClient.clear();
  vi.resetAllMocks();
  currentUrl = new URL(JOBS_URL);
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

/** The server layout gives the page a bounded flex pane. */
function renderJobsPage(width = 960) {
  const screen = render(JobsPage);
  screen.container.style.cssText = `display:flex; flex-direction:column; height:520px; width:${width}px`;
  return screen;
}

function job(id: string, changes: Partial<AdminJob> = {}): AdminJob {
  return {
    id,
    type: 'webhook',
    status: JobStatus.FAILED,
    attempts: 3,
    maxAttempts: 5,
    failure: 'http_error',
    retry: JobRetryAvailability.ALLOWED,
    version: 'a'.repeat(64),
    createdAt: new Date('2026-11-10T12:00:00Z'),
    updatedAt: null,
    nextAttemptAt: null,
    ...changes
  };
}

describe('job queue list', () => {
  it('shows load errors and the empty state without a manual refresh', async () => {
    mocks.listJobs.mockRejectedValueOnce(new ConnectError('denied', Code.PermissionDenied));
    const screen = renderJobsPage();
    await expect.element(screen.getByText('You do not have permission to do that.')).toBeVisible();
    // A failed load is not an empty queue.
    expect(screen.container.textContent).not.toContain('No jobs yet');
    expect(screen.getByRole('button', { name: 'Refresh' }).query()).toBeNull();
    screen.unmount();

    queryClient.clear();
    mocks.listJobs.mockResolvedValue({ jobs: [], nextCursor: '' });
    const empty = renderJobsPage();
    await expect.element(empty.getByText('No jobs yet')).toBeVisible();
  });

  it('shows the attempt count alone when the job has no attempt limit', async () => {
    mocks.listJobs.mockResolvedValue({
      jobs: [job('unlimited', { attempts: 7, maxAttempts: undefined })],
      nextCursor: ''
    });
    const screen = renderJobsPage();
    await expect.element(screen.getByRole('cell', { name: '7', exact: true })).toBeVisible();
    expect(screen.container.textContent).not.toContain('7 /');
  });

  it('shows a compatibility notice instead of jobs on a server without a job queue', async () => {
    createTestServerScope({ api: mocks, serverInfo: { limitedFeatures: ['backgroundJobs'] } });
    const screen = renderJobsPage();
    await expect.element(screen.getByRole('link', { name: 'View compatibility' })).toBeVisible();
    expect(screen.container.querySelector('table')).toBeNull();
    expect(mocks.listJobs).not.toHaveBeenCalled();
  });

  it('renders one linked row per job and loads the next page from the table sentinel', async () => {
    mocks.listJobs
      .mockResolvedValueOnce({ jobs: [job('first')], nextCursor: 'next' })
      .mockResolvedValueOnce({
        jobs: [
          job('first'),
          job('second', { type: 'asset_cleanup', status: JobStatus.QUEUED, failure: '' })
        ],
        nextCursor: ''
      });
    const screen = renderJobsPage();
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
    expect(mocks.listJobs.mock.calls[1][0]).toMatchObject({ cursor: 'next' });
  });

  it('retries a failed job from its row and shows activity for running jobs', async () => {
    mocks.listJobs
      .mockResolvedValueOnce({
        jobs: [job('failed'), job('expired', { retry: JobRetryAvailability.EXPIRED })],
        nextCursor: ''
      })
      .mockResolvedValue({
        jobs: [
          job('failed', { status: JobStatus.RUNNING, retry: JobRetryAvailability.NOT_FAILED }),
          job('expired', { retry: JobRetryAvailability.EXPIRED })
        ],
        nextCursor: ''
      });
    mocks.retryJob.mockResolvedValue(
      job('failed', { status: JobStatus.QUEUED, retry: JobRetryAvailability.NOT_FAILED })
    );
    const screen = renderJobsPage();
    const retries = screen.getByRole('button', { name: 'Try Again' });
    await expect.element(retries.nth(0)).toBeEnabled();
    await expect.element(retries.nth(1)).toBeDisabled();
    await retries.nth(0).click();
    expect(mocks.retryJob).toHaveBeenCalledExactlyOnceWith('failed', 'a'.repeat(64));
    await expect.element(screen.getByText('Running', { exact: true })).toBeVisible();
    expect(screen.container.querySelectorAll('[data-activity-indicator]')).toHaveLength(1);
    expect(screen.container.querySelectorAll('button')).toHaveLength(2);
  });

  it('deletes a failed job from its row after confirmation', async () => {
    mocks.listJobs
      .mockResolvedValueOnce({
        jobs: [
          job('doomed', { failure: 'doomed_failure', version: 'd'.repeat(64) }),
          job('expired', { retry: JobRetryAvailability.EXPIRED })
        ],
        nextCursor: ''
      })
      .mockResolvedValue({
        jobs: [job('expired', { retry: JobRetryAvailability.EXPIRED })],
        nextCursor: ''
      });
    mocks.deleteJob.mockResolvedValue(undefined);
    const screen = renderJobsPage();
    const deletes = screen.getByRole('button', { name: 'Delete' });
    // An expired job cannot be retried, but it can be deleted.
    await expect.element(deletes.nth(1)).toBeEnabled();
    await deletes.nth(0).click();
    const dialog = screen.getByRole('dialog');
    await expect.element(dialog.getByText('Delete this job?')).toBeVisible();
    await expect
      .element(dialog.getByText('The work of this job is abandoned. You cannot retry it later.'))
      .toBeVisible();
    await dialog.getByRole('button', { name: 'Delete' }).click();
    expect(mocks.deleteJob).toHaveBeenCalledExactlyOnceWith('doomed', 'd'.repeat(64));
    await expect.element(screen.getByText('doomed_failure')).not.toBeInTheDocument();
    await expect.element(screen.getByRole('dialog')).not.toBeInTheDocument();
    expect(screen.container.querySelectorAll('a.data-table-row-link')).toHaveLength(1);
  });

  it('keeps the job when the deletion is cancelled', async () => {
    mocks.listJobs.mockResolvedValue({ jobs: [job('kept')], nextCursor: '' });
    const screen = renderJobsPage();
    await screen.getByRole('button', { name: 'Delete' }).click();
    const dialog = screen.getByRole('dialog');
    await dialog.getByRole('button', { name: 'Cancel' }).click();
    await expect.element(screen.getByRole('dialog')).not.toBeInTheDocument();
    expect(mocks.deleteJob).not.toHaveBeenCalled();
    await expect.element(screen.getByRole('link', { name: 'Webhook delivery' })).toBeVisible();
  });

  it('reloads the list when another operator changed the job before the delete', async () => {
    mocks.listJobs
      .mockResolvedValueOnce({ jobs: [job('changed')], nextCursor: '' })
      .mockResolvedValue({
        jobs: [
          job('changed', { status: JobStatus.QUEUED, retry: JobRetryAvailability.NOT_FAILED })
        ],
        nextCursor: ''
      });
    mocks.deleteJob.mockRejectedValue(new ConnectError('changed', Code.FailedPrecondition));
    const screen = renderJobsPage();
    await screen.getByRole('button', { name: 'Delete' }).click();
    await screen.getByRole('dialog').getByRole('button', { name: 'Delete' }).click();
    await expect.element(screen.getByRole('cell', { name: 'Queued' })).toBeVisible();
    expect(screen.getByRole('button', { name: 'Delete' }).query()).toBeNull();
  });

  it('treats a job that is already gone as deleted', async () => {
    mocks.listJobs
      .mockResolvedValueOnce({ jobs: [job('gone')], nextCursor: '' })
      .mockResolvedValue({ jobs: [], nextCursor: '' });
    mocks.deleteJob.mockRejectedValue(new ConnectError('missing', Code.NotFound));
    const screen = renderJobsPage();
    await screen.getByRole('button', { name: 'Delete' }).click();
    await screen.getByRole('dialog').getByRole('button', { name: 'Delete' }).click();
    await expect.element(screen.getByText('No jobs yet')).toBeVisible();
    // Another operator deleted it first. That is not an error for this one.
    expect(toast.error).not.toHaveBeenCalled();
  });

  it('offers no delete for a job that is not failed', async () => {
    mocks.listJobs.mockResolvedValue({
      jobs: [job('queued', { status: JobStatus.QUEUED, retry: JobRetryAvailability.NOT_FAILED })],
      nextCursor: ''
    });
    const screen = renderJobsPage();
    await expect.element(screen.getByRole('link', { name: 'Webhook delivery' })).toBeVisible();
    expect(screen.getByRole('button', { name: 'Delete' }).query()).toBeNull();
  });

  it.each([390, 960])('keeps the table viewport stable across filters at %i px', async (width) => {
    globalThis.IntersectionObserver = originalIntersectionObserver;
    const queued = Promise.withResolvers<{ jobs: AdminJob[]; nextCursor: string }>();
    mocks.goto.mockImplementation((url: string) => {
      currentUrl = new URL(url, JOBS_URL);
    });
    mocks.listJobs.mockImplementation(({ status, cursor }) => {
      if (status === JobStatus.QUEUED) return queued.promise;
      if (cursor === 'more') return { jobs: [job('extra')], nextCursor: '' };
      return { jobs: Array.from({ length: 50 }, (_, i) => job(String(i))), nextCursor: 'more' };
    });
    const screen = renderJobsPage(width);
    await expect
      .element(screen.getByRole('link', { name: 'Webhook delivery' }).first())
      .toBeVisible();
    const viewport = screen.container.querySelector<HTMLElement>(
      '.data-table-viewport [role="region"]'
    )!;
    const header = screen.container.querySelector('thead')!;
    const columnWidths = () => [...header.querySelectorAll('th')].map((th) => th.offsetWidth);
    const height = viewport.clientHeight;
    const columns = columnWidths();
    expect(height).toBeGreaterThan(100);
    expect(viewport.scrollHeight).toBeGreaterThan(height);
    viewport.scrollTop = viewport.scrollHeight;
    await expect
      .poll(() => screen.container.querySelectorAll('.data-table-row-link').length)
      .toBe(51);
    expect(header.getBoundingClientRect().top).toBeCloseTo(viewport.getBoundingClientRect().top, 0);
    await screen.getByRole('radio', { name: 'Queued' }).click();
    await expect.element(screen.getByRole('radio', { name: 'Queued' })).toBeChecked();
    await expect
      .poll(() => screen.container.querySelectorAll('.data-table-row-link').length)
      .toBe(0);
    expect(viewport.clientHeight).toBe(height);
    expect(columnWidths()).toEqual(columns);
    queued.resolve({ jobs: [], nextCursor: '' });
    await expect.element(screen.getByText('No jobs yet')).toBeVisible();
    expect(viewport.clientHeight).toBe(height);
    expect(columnWidths()).toEqual(columns);
  });

  it('keeps the selected status filter in the URL', async () => {
    mocks.listJobs.mockResolvedValue({ jobs: [job('failed')], nextCursor: '' });
    const screen = renderJobsPage();
    await expect.element(screen.getByRole('link', { name: 'Webhook delivery' })).toBeVisible();
    expect(mocks.listJobs.mock.calls[0][0]).toMatchObject({ status: JobStatus.UNSPECIFIED });
    await screen.getByRole('radio', { name: 'Failed' }).click();
    expect(mocks.goto).toHaveBeenCalledExactlyOnceWith('/chat/-/manage/server/jobs?status=failed', {
      replaceState: true,
      keepFocus: true,
      noScroll: true
    });
  });

  it('requests the status filter of the URL and clears it for All', async () => {
    currentUrl = new URL(`${JOBS_URL}?status=failed`);
    mocks.listJobs.mockResolvedValue({ jobs: [job('failed')], nextCursor: '' });
    const screen = renderJobsPage();
    await expect.element(screen.getByRole('radio', { name: 'Failed' })).toBeChecked();
    expect(mocks.listJobs.mock.calls[0][0]).toMatchObject({ status: JobStatus.FAILED });
    await screen.getByRole('radio', { name: 'All' }).click();
    expect(mocks.goto).toHaveBeenCalledExactlyOnceWith('/chat/-/manage/server/jobs', {
      replaceState: true,
      keepFocus: true,
      noScroll: true
    });
  });

  it('lists all jobs for an unknown status filter', async () => {
    currentUrl = new URL(`${JOBS_URL}?status=succeeded`);
    mocks.listJobs.mockResolvedValue({ jobs: [job('failed')], nextCursor: '' });
    const screen = renderJobsPage();
    await expect.element(screen.getByRole('radio', { name: 'All' })).toBeChecked();
    expect(mocks.listJobs.mock.calls[0][0]).toMatchObject({ status: JobStatus.UNSPECIFIED });
  });
});
