/** Administrative job reads and retry commands. Payloads stay on the server. */
import {
  Code,
  ConnectError,
  createChattoClient,
  type ConnectAPIConfig
} from '@chatto/client/api/connect';
import { JobsService } from '@chatto/api-types/admin/v1/jobs_connect';
import { type Job, JobStatus, RetryJobState } from '@chatto/api-types/admin/v1/jobs_pb';

export { JobStatus, RetryJobState };

/** A safe job summary, with timestamps normalized at the API boundary. */
export type AdminJob = {
  id: string;
  type: string;
  status: JobStatus;
  attempts: number;
  maxAttempts: number;
  createdAt: Date | null;
  updatedAt: Date | null;
  nextAttemptAt: Date | null;
  failure: string;
  retryState: RetryJobState | undefined;
  /** Opaque retry budget version. Echo it unchanged when retrying this snapshot. */
  retryVersion: string;
};

/** One page of jobs in the server's stable listing order. */
export type AdminJobsPage = { jobs: AdminJob[]; nextPageToken: string };

/** Creates an API for one authenticated server connection. */
export function createAdminJobsAPI(config: ConnectAPIConfig) {
  const client = createChattoClient(JobsService, config);
  return {
    async listJobs(
      input: { pageSize?: number; pageToken?: string } = {},
      options: { signal?: AbortSignal } = {}
    ): Promise<AdminJobsPage> {
      const response = await client.listJobs(
        { pageSize: input.pageSize ?? 50, pageToken: input.pageToken ?? '' },
        { signal: options.signal }
      );
      return { jobs: response.jobs.map(adminJob), nextPageToken: response.nextPageToken };
    },
    async getJob(id: string, options: { signal?: AbortSignal } = {}): Promise<AdminJob | null> {
      try {
        const response = await client.getJob({ id }, { signal: options.signal });
        return response.job ? adminJob(response.job) : null;
      } catch (error) {
        if (error instanceof ConnectError && error.code === Code.NotFound) return null;
        throw error;
      }
    },
    async retryJob(
      id: string,
      retryVersion: string,
      options: { signal?: AbortSignal } = {}
    ): Promise<AdminJob> {
      const response = await client.retryJob({ id, retryVersion }, { signal: options.signal });
      if (!response.job) throw new ConnectError('Missing job response', Code.Internal);
      return adminJob(response.job);
    }
  };
}

function adminJob(job: Job): AdminJob {
  return {
    id: job.id,
    type: job.type,
    status: job.status,
    attempts: job.attempts,
    maxAttempts: job.maxAttempts,
    createdAt: job.createdAt?.toDate() ?? null,
    updatedAt: job.updatedAt?.toDate() ?? null,
    nextAttemptAt: job.nextAttemptAt?.toDate() ?? null,
    failure: job.failure,
    retryState: job.retryState,
    retryVersion: job.retryVersion
  };
}
