import { Timestamp } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { AdminJobService } from '@chatto/api-types/admin/v1/jobs_connect';
import { JobRetryAvailability, JobStatus } from '@chatto/api-types/admin/v1/jobs_pb';
import { fakeServer, mockService, receivedRequest } from '@chatto/client/testing/fakeServer';
import { createAdminJobsAPI } from './adminJobs';

const mocks = mockService(AdminJobService);
const config = () => fakeServer((router) => router.service(AdminJobService, mocks));

beforeEach(() => vi.resetAllMocks());

describe('administrative jobs API', () => {
  it('preserves opaque cursors, retry availability, and safe job metadata', async () => {
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
          retry: JobRetryAvailability.EXPIRED,
          version: 'a'.repeat(64)
        }
      ],
      nextCursor: 'opaque-next'
    });
    const page = await createAdminJobsAPI(config()).listJobs({
      pageSize: 20,
      cursor: 'opaque-before'
    });
    expect(receivedRequest(mocks.listJobs)).toMatchObject({
      pageSize: 20,
      cursor: 'opaque-before'
    });
    expect(page.nextCursor).toBe('opaque-next');
    expect(page.jobs[0]).toMatchObject({
      id: 'opaque-id',
      status: JobStatus.FAILED,
      maxAttempts: 4,
      failure: 'transport_error',
      createdAt: new Date('2026-11-10T12:00:00Z'),
      updatedAt: null,
      nextAttemptAt: null,
      retry: JobRetryAvailability.EXPIRED,
      version: 'a'.repeat(64)
    });
  });

  it('keeps absent or unknown retry availability rather than inferring permission', async () => {
    mocks.listJobs.mockReturnValue({
      jobs: [
        { id: 'old', status: JobStatus.FAILED },
        { id: 'new', status: JobStatus.FAILED, retry: 987 as JobRetryAvailability }
      ]
    });
    const page = await createAdminJobsAPI(config()).listJobs();
    expect(page.jobs[0].retry).toBeUndefined();
    expect(page.jobs[1].retry).toBe(987);
  });

  it('reports an absent attempt limit as unlimited', async () => {
    mocks.listJobs.mockReturnValue({
      jobs: [{ id: 'unlimited', status: JobStatus.QUEUED, attempts: 7 }]
    });
    const page = await createAdminJobsAPI(config()).listJobs();
    expect(page.jobs[0]).toMatchObject({ attempts: 7, maxAttempts: undefined });
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
    await expect(api.retryJob('job', 'b'.repeat(64))).resolves.toMatchObject({
      id: 'job',
      status: JobStatus.QUEUED
    });
    expect(receivedRequest(mocks.retryJob)).toMatchObject({
      id: 'job',
      version: 'b'.repeat(64)
    });
    mocks.retryJob.mockReturnValue({});
    await expect(api.retryJob('job', 'b'.repeat(64))).rejects.toMatchObject({
      code: Code.Internal
    });
  });

  it('deletes the job state that the caller read and keeps a rejected delete', async () => {
    mocks.deleteJob.mockReturnValue({});
    const api = createAdminJobsAPI(config());
    await expect(api.deleteJob('job', 'c'.repeat(64))).resolves.toBeUndefined();
    expect(receivedRequest(mocks.deleteJob)).toMatchObject({ id: 'job', version: 'c'.repeat(64) });
    mocks.deleteJob.mockImplementation(() => {
      throw new ConnectError('changed', Code.FailedPrecondition);
    });
    await expect(api.deleteJob('job', 'c'.repeat(64))).rejects.toMatchObject({
      code: Code.FailedPrecondition
    });
  });
});
