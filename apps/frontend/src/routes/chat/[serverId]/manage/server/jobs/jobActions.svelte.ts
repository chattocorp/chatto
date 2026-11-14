/** Manual job retry and delete that the job list rows and the job detail page share. */
import {
  createAdminJobsAPI,
  JobRetryAvailability,
  JobStatus,
  type AdminJob,
  type AdminJobsPage
} from '$lib/api/adminJobs';
import { m } from '$lib/i18n/messages';
import { adminQueryKeys } from '$lib/query/admin';
import { createMutation, queryClient } from '$lib/query/client';
import type { ServerScope } from '$lib/state/server/scope.svelte';
import { createSessionGuard, type SessionSnapshot } from '$lib/state/server/sessionGuard.svelte';
import { toastError } from '$lib/utils/errorMessage';

/** The job state that the operator saw when starting the action. */
type JobVariables = SessionSnapshot & { id: string; version: string };

/** Tells whether the server offers a retry for this rendered job snapshot. */
export function canRetryJob(job: AdminJob): boolean {
  return job.retry === JobRetryAvailability.ALLOWED && job.version !== '';
}

/**
 * Tells whether this rendered job snapshot can be deleted. Every failed job
 * can, also when retry is not available.
 */
export function canDeleteJob(job: AdminJob): boolean {
  return job.status === JobStatus.FAILED && job.version !== '';
}

/**
 * Creates the retry and delete actions for one page. A successful retry
 * updates the job in the list and detail caches, then reloads both. A
 * successful delete removes the job from these caches, reloads the list, and
 * calls `onDeleted`. A rejected action reloads the list and the job, because
 * another worker or operator may have changed the job.
 *
 * Call it while a component initializes.
 */
export function createJobActions(
  scope: ServerScope,
  options: { onDeleted?: (id: string) => void } = {}
) {
  const session = createSessionGuard(scope);
  const connection = scope.connection;
  const api = connection.getAPI(createAdminJobsAPI);
  const listKey = adminQueryKeys.jobs(scope.serverId, connection);
  const jobKey = (id: string) => adminQueryKeys.job(scope.serverId, connection, id);

  function reloadList() {
    // The prefix covers the list under every status filter.
    void queryClient.invalidateQueries({ queryKey: listKey });
  }

  function reload(id: string) {
    reloadList();
    void queryClient.invalidateQueries({ queryKey: jobKey(id), exact: true });
  }

  /** Changes the cached jobs of the list under every status filter. */
  function updateListedJobs(update: (jobs: AdminJob[]) => AdminJob[]) {
    queryClient.setQueriesData<{ pages: AdminJobsPage[]; pageParams: string[] }>(
      { queryKey: listKey },
      (current) =>
        current && {
          ...current,
          pages: current.pages.map((page) => ({ ...page, jobs: update(page.jobs) }))
        }
    );
  }

  const retryMutation = createMutation(() => ({
    mutationFn: ({ id, version }: JobVariables) => api.retryJob(id, version),
    onSuccess: (job, variables) => {
      if (!session.isCurrent(variables)) return;
      queryClient.setQueryData(jobKey(job.id), job);
      updateListedJobs((jobs) => jobs.map((entry) => (entry.id === job.id ? job : entry)));
      reload(job.id);
    },
    onError: (error, variables) => {
      if (!session.isCurrent(variables)) return;
      toastError(error, m('admin.jobs.retry_failed'));
      reload(variables.id);
    }
  }));

  const deleteMutation = createMutation(() => ({
    mutationFn: ({ id, version }: JobVariables) => api.deleteJob(id, version),
    onSuccess: (_response, variables) => {
      if (!session.isCurrent(variables)) return;
      // A deleted job reads as null, the same as a job that the server does not find.
      queryClient.setQueryData(jobKey(variables.id), null);
      updateListedJobs((jobs) => jobs.filter((entry) => entry.id !== variables.id));
      reloadList();
      options.onDeleted?.(variables.id);
    },
    onError: (error, variables) => {
      if (!session.isCurrent(variables)) return;
      toastError(error, m('admin.jobs.delete_failed'));
      reload(variables.id);
    }
  }));

  function variables(job: AdminJob): JobVariables {
    return { ...session.snapshot(), id: job.id, version: job.version };
  }

  const pending = () => retryMutation.isPending || deleteMutation.isPending;

  return {
    /** True while any action of this page is pending. One action runs at a time. */
    get pending() {
      return pending();
    },
    /** True while the retry of this job is pending. */
    retrying(job: AdminJob) {
      return retryMutation.isPending && retryMutation.variables?.id === job.id;
    },
    /** True while the delete of this job is pending. */
    deleting(job: AdminJob) {
      return deleteMutation.isPending && deleteMutation.variables?.id === job.id;
    },
    /** Retries the rendered job state only; the server rejects a changed job. */
    retry(job: AdminJob) {
      if (pending() || !canRetryJob(job)) return;
      retryMutation.mutate(variables(job));
    },
    /**
     * Deletes the rendered failed job; the server rejects a changed job. The
     * work is abandoned, so ask the operator to confirm first.
     */
    delete(job: AdminJob) {
      if (pending() || !canDeleteJob(job)) return;
      deleteMutation.mutate(variables(job));
    }
  };
}
