import { Timestamp } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { JobsService } from '@chatto/api-types/admin/v1/jobs_connect';
import { JobStatus, RetryJobState } from '@chatto/api-types/admin/v1/jobs_pb';
import { fakeServer, mockService, receivedRequest } from '@chatto/client/testing/fakeServer';
import { createAdminJobsAPI } from './adminJobs';

const mocks = mockService(JobsService);
const config = () => fakeServer((router) => router.service(JobsService, mocks));

beforeEach(() => vi.resetAllMocks());

describe('administrative jobs API', () => {
  it('preserves opaque page tokens, retry states, and safe job metadata', async () => {
    mocks.listJobs.mockReturnValue({
      jobs: [
        {
          id: 'opaque-id',
          type: 'webhook',
          status: JobStatus.FAILED,
          attempts: 4,
          maxAttempts: 4,
          failure: 'transport_error',
          createdAt: Timestamp.fromDate(new Date('2026-11-10T12:00:00Z')),
          retryState: RetryJobState.EXPIRED
        }
      ],
      nextPageToken: 'opaque-next'
    });
    const page = await createAdminJobsAPI(config()).listJobs({
      pageSize: 20,
      pageToken: 'opaque-before'
    });
    expect(receivedRequest(mocks.listJobs)).toMatchObject({
      pageSize: 20,
      pageToken: 'opaque-before'
    });
    expect(page.nextPageToken).toBe('opaque-next');
    expect(page.jobs[0]).toMatchObject({
      id: 'opaque-id',
      status: JobStatus.FAILED,
      failure: 'transport_error',
      createdAt: new Date('2026-11-10T12:00:00Z'),
      updatedAt: null,
      nextAttemptAt: null,
      retryState: RetryJobState.EXPIRED
    });
  });

  it('keeps an absent or unknown retry state disabled rather than inferring permission', async () => {
    mocks.listJobs.mockReturnValue({
      jobs: [
        { id: 'old', status: JobStatus.FAILED },
        { id: 'new', status: JobStatus.FAILED, retryState: 987 as RetryJobState }
      ]
    });
    const page = await createAdminJobsAPI(config()).listJobs();
    expect(page.jobs[0].retryState).toBeUndefined();
    expect(page.jobs[1].retryState).toBe(987);
  });

  it('reports missing jobs as null but keeps authorization failures', async () => {
    mocks.getJob.mockImplementation(() => {
      throw new ConnectError('missing', Code.NotFound);
    });
    const api = createAdminJobsAPI(config());
    await expect(api.getJob('missing')).resolves.toBeNull();
    mocks.getJob.mockImplementation(() => {
      throw new ConnectError('denied', Code.PermissionDenied);
    });
    await expect(api.getJob('private')).rejects.toMatchObject({ code: Code.PermissionDenied });
  });

  it('returns the canonical job after retry and rejects an empty success response', async () => {
    mocks.retryJob.mockReturnValue({ job: { id: 'job', status: JobStatus.QUEUED } });
    const api = createAdminJobsAPI(config());
    await expect(api.retryJob('job')).resolves.toMatchObject({
      id: 'job',
      status: JobStatus.QUEUED
    });
    expect(receivedRequest(mocks.retryJob)).toMatchObject({ id: 'job' });
    mocks.retryJob.mockReturnValue({});
    await expect(api.retryJob('job')).rejects.toMatchObject({ code: Code.Internal });
  });
});
