/** Administrative job reads, and retry and delete commands. Payloads stay on the server. */
import {
  Code,
  ConnectError,
  createChattoClient,
  type ConnectAPIConfig
} from '@chatto/client/api/connect';
import { AdminJobService } from '@chatto/api-types/admin/v1/jobs_connect';
import { type Job, JobRetryAvailability, JobStatus } from '@chatto/api-types/admin/v1/jobs_pb';

export { JobRetryAvailability, JobStatus };

/** A safe job summary, with timestamps normalized at the API boundary. */
export type AdminJob = {
  id: string;
  type: string;
  status: JobStatus;
  attempts: number;
  /** Automatic attempt limit. Undefined when the job retries without a limit. */
  maxAttempts: number | undefined;
  createdAt: Date | null;
  updatedAt: Date | null;
  nextAttemptAt: Date | null;
  failure: string;
  /** Retry availability. Undefined when the server did not report it. */
  retry: JobRetryAvailability | undefined;
  /**
   * Opaque version of this job state. Echo it unchanged when retrying or
   * deleting this snapshot; the server rejects the command if the job changed.
   */
  version: string;
};

/** One page of jobs in the server's stable listing order. */
export type AdminJobsPage = {
  jobs: AdminJob[];
  /** Opaque cursor of the next page. Empty when no more jobs follow. */
  nextCursor: string;
};

/** Creates an API for one authenticated server connection. */
export function createAdminJobsAPI(config: ConnectAPIConfig) {
  const client = createChattoClient(AdminJobService, config);
  return {
    async listJobs(
      input: { pageSize?: number; cursor?: string; status?: JobStatus } = {},
      options: { signal?: AbortSignal } = {}
    ): Promise<AdminJobsPage> {
      const response = await client.listJobs(
        {
          pageSize: input.pageSize ?? 50,
          cursor: input.cursor ?? '',
          status: input.status ?? JobStatus.UNSPECIFIED
        },
        { signal: options.signal }
      );
      return { jobs: response.jobs.map(adminJob), nextCursor: response.nextCursor };
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
      version: string,
      options: { signal?: AbortSignal } = {}
    ): Promise<AdminJob> {
      const response = await client.retryJob({ id, version }, { signal: options.signal });
      if (!response.job) throw new ConnectError('Missing job response', Code.Internal);
      return adminJob(response.job);
    },
    /**
     * Deletes a failed job. Its work is abandoned and cannot be retried later.
     * The server rejects the command when the job is not failed or changed
     * after `version` was read, and reports an already deleted job as not found.
     */
    async deleteJob(
      id: string,
      version: string,
      options: { signal?: AbortSignal } = {}
    ): Promise<void> {
      await client.deleteJob({ id, version }, { signal: options.signal });
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
    retry: job.retry,
    version: job.version
  };
}
