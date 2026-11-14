/** Manual job retry that the job list rows and the job detail page share. */
import {
  createAdminJobsAPI,
  RetryJobState,
  type AdminJob,
  type AdminJobsPage
} from '$lib/api/adminJobs';
import { m } from '$lib/i18n/messages';
import { adminQueryKeys } from '$lib/query/admin';
import { createMutation, queryClient } from '$lib/query/client';
import type { ServerScope } from '$lib/state/server/scope.svelte';
import { createSessionGuard, type SessionSnapshot } from '$lib/state/server/sessionGuard.svelte';
import { toastError } from '$lib/utils/errorMessage';

type RetryVariables = SessionSnapshot & { id: string; retryVersion: string };

/** Tells whether the server offers a retry for this rendered job snapshot. */
export function canRetryJob(job: AdminJob): boolean {
  return job.retryState === RetryJobState.ALLOWED && job.retryVersion !== '';
}

/**
 * Creates the retry action for one page. A successful retry updates the job in
 * the list and detail caches, then reloads both. A rejected retry reloads them,
 * because another worker or operator may have changed the job.
 *
 * Call it while a component initializes.
 */
export function createJobRetry(scope: ServerScope) {
  const session = createSessionGuard(scope);
  const connection = scope.connection;
  const api = connection.getAPI(createAdminJobsAPI);
  const listKey = adminQueryKeys.jobs(scope.serverId, connection);
  const jobKey = (id: string) => adminQueryKeys.job(scope.serverId, connection, id);

  function reload(id: string) {
    // The prefix covers the list under every status filter.
    void queryClient.invalidateQueries({ queryKey: listKey });
    void queryClient.invalidateQueries({ queryKey: jobKey(id), exact: true });
  }

  const mutation = createMutation(() => ({
    mutationFn: ({ id, retryVersion }: RetryVariables) => api.retryJob(id, retryVersion),
    onSuccess: (job, variables) => {
      if (!session.isCurrent(variables)) return;
      queryClient.setQueryData(jobKey(job.id), job);
      queryClient.setQueriesData<{ pages: AdminJobsPage[]; pageParams: string[] }>(
        { queryKey: listKey },
        (current) =>
          current && {
            ...current,
            pages: current.pages.map((page) => ({
              ...page,
              jobs: page.jobs.map((entry) => (entry.id === job.id ? job : entry))
            }))
          }
      );
      reload(job.id);
    },
    onError: (error, variables) => {
      if (!session.isCurrent(variables)) return;
      toastError(error, m('admin.jobs.retry_failed'));
      reload(variables.id);
    }
  }));

  return {
    /** True while any retry of this page is pending. One retry runs at a time. */
    get pending() {
      return mutation.isPending;
    },
    /** True while the retry of this job is pending. */
    retrying(job: AdminJob) {
      return mutation.isPending && mutation.variables?.id === job.id;
    },
    /** Retries the rendered attempt budget only; a later budget is rejected. */
    retry(job: AdminJob) {
      if (mutation.isPending || !canRetryJob(job)) return;
      mutation.mutate({ ...session.snapshot(), id: job.id, retryVersion: job.retryVersion });
    }
  };
}
