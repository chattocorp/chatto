<!-- @component Server job queue with safe details and retries for failed jobs. -->
<script lang="ts">
  import { SvelteMap } from 'svelte/reactivity';
  import {
    createAdminJobsAPI,
    JobStatus,
    RetryJobState,
    type AdminJob,
    type AdminJobsPage
  } from '$lib/api/adminJobs';
  import { m } from '$lib/i18n/messages';
  import { getLocale } from '$lib/i18n/runtime';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { createSessionGuard, type SessionSnapshot } from '$lib/state/server/sessionGuard.svelte';
  import { adminQueryKeys } from '$lib/query/admin';
  import { createInfiniteQuery, createMutation, queryClient } from '$lib/query/client';
  import {
    EmptyState,
    Hint,
    LoadingFog,
    PageTitle,
    PaneContent,
    PaneHeader,
    Panel,
    Pill
  } from '$lib/ui';
  import { Button } from '$lib/ui/form';
  import { errorMessage, toastError } from '$lib/utils/errorMessage';
  import { formatDateTime, timeFormatSettingsFor } from '$lib/utils/formatTime';

  const scope = useServerScope();
  const session = createSessionGuard(scope);
  const connection = scope.connection;
  const queryKey = adminQueryKeys.jobs(scope.serverId, connection);
  const api = connection.getAPI(createAdminJobsAPI);
  const timeSettings = $derived(timeFormatSettingsFor(scope.store.currentUser.user?.settings));
  const locale = $derived(getLocale());

  const jobsQuery = createInfiniteQuery(() => ({
    queryKey,
    queryFn: ({ pageParam, signal }) => api.listJobs({ pageToken: pageParam }, { signal }),
    initialPageParam: '',
    getNextPageParam: (lastPage) => lastPage.nextPageToken || undefined,
    refetchOnMount: 'always' as const
  }));

  // Pages may overlap while running jobs change. Keep one row for each stable ID.
  const jobs = $derived.by(() => {
    const byId = new SvelteMap<string, AdminJob>();
    for (const page of jobsQuery.data?.pages ?? []) {
      for (const job of page.jobs) byId.set(job.id, job);
    }
    return [...byId.values()];
  });
  const loadError = $derived(
    jobsQuery.error ? errorMessage(jobsQuery.error, m('admin.jobs.load_failed')) : null
  );

  type RetryVariables = SessionSnapshot & { id: string };
  const retryMutation = createMutation(() => ({
    mutationFn: ({ id }: RetryVariables) => api.retryJob(id),
    onSuccess: (job, variables) => {
      if (!session.isCurrent(variables)) return;
      queryClient.setQueryData<{ pages: AdminJobsPage[]; pageParams: string[] }>(
        queryKey,
        (current) =>
          current
            ? {
                ...current,
                pages: current.pages.map((page) => ({
                  ...page,
                  jobs: page.jobs.map((entry) => (entry.id === job.id ? job : entry))
                }))
              }
            : current
      );
      void queryClient.invalidateQueries({ queryKey, exact: true });
    },
    onError: (error, variables) => {
      if (!session.isCurrent(variables)) return;
      toastError(error, m('admin.jobs.retry_failed'));
      // A different worker or operator may have changed this job in the meantime.
      void queryClient.invalidateQueries({ queryKey, exact: true });
    }
  }));

  function retry(job: AdminJob): void {
    if (retryMutation.isPending || job.retryState !== RetryJobState.ALLOWED) return;
    retryMutation.mutate({ ...session.snapshot(), id: job.id });
  }

  function jobType(job: AdminJob): string {
    switch (job.type) {
      case 'webhook':
        return m('admin.jobs.webhook');
      case 'media':
        return m('admin.jobs.media');
      default:
        return m('common.unknown');
    }
  }

  function jobStatus(status: JobStatus): string {
    switch (status) {
      case JobStatus.QUEUED:
        return m('admin.jobs.queued');
      case JobStatus.RUNNING:
        return m('admin.jobs.running');
      case JobStatus.SUCCEEDED:
        return m('admin.jobs.succeeded');
      case JobStatus.FAILED:
        return m('admin.jobs.failed');
      default:
        return m('common.unknown');
    }
  }

  function jobTone(status: JobStatus): 'neutral' | 'action' | 'success' | 'danger' {
    switch (status) {
      case JobStatus.RUNNING:
        return 'action';
      case JobStatus.SUCCEEDED:
        return 'success';
      case JobStatus.FAILED:
        return 'danger';
      default:
        return 'neutral';
    }
  }

  function timestamp(value: Date | null): string {
    return value ? formatDateTime(value.toISOString(), timeSettings, locale) : '—';
  }
</script>

<PageTitle title={m('admin.common.page_title', { title: m('admin.jobs.title') })} />

<div class="pane-page">
  <PaneHeader title={m('admin.jobs.title')} subtitle={m('admin.jobs.subtitle')} />
  <PaneContent>
    <Panel title={m('admin.jobs.title')} icon="iconify icon-[uil--process]">
      <div class="flex flex-col gap-4">
        <div class="flex justify-end">
          <Button
            variant="secondary"
            onclick={() => jobsQuery.refetch()}
            disabled={jobsQuery.isFetching}
          >
            <span aria-hidden="true" class="iconify icon-[uil--sync]"></span>
            {m('admin.system.refresh')}
          </Button>
        </div>
        {#if loadError}
          <Hint tone="danger">{loadError}</Hint>
        {/if}
        {#if jobsQuery.isPending}
          <LoadingFog class="h-48" label={m('common.loading')} />
        {:else if jobs.length === 0 && !loadError}
          <EmptyState icon="icon-[uil--check-circle]" title={m('admin.jobs.empty')} />
        {:else}
          <ul class="flex flex-col divide-y divide-border" aria-label={m('admin.jobs.title')}>
            {#each jobs as job (job.id)}
              <li class="py-4 first:pt-0 last:pb-0">
                <details>
                  <summary
                    class="cursor-pointer rounded px-1 py-2 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-action"
                  >
                    <span class="font-medium">{jobType(job)}</span>
                    <span class="ms-3 inline-block align-middle">
                      <Pill tone={jobTone(job.status)}>{jobStatus(job.status)}</Pill>
                    </span>
                    <span class="mt-2 block text-sm text-muted sm:ms-4 sm:mt-0 sm:inline">
                      {timestamp(job.createdAt)}
                    </span>
                  </summary>
                  <div class="flex flex-col gap-4 pt-4">
                    <dl class="grid min-w-0 grid-cols-1 gap-x-6 gap-y-3 sm:grid-cols-2">
                      <div class="min-w-0">
                        <dt class="text-sm text-muted">{m('admin.common.id')}</dt>
                        <dd class="font-mono text-sm break-all">{job.id}</dd>
                      </div>
                      <div>
                        <dt class="text-sm text-muted">{m('admin.jobs.attempts')}</dt>
                        <dd>
                          {job.attempts.toLocaleString(locale)} / {job.maxAttempts.toLocaleString(
                            locale
                          )}
                        </dd>
                      </div>
                      <div>
                        <dt class="text-sm text-muted">{m('admin.jobs.updated')}</dt>
                        <dd>{timestamp(job.updatedAt)}</dd>
                      </div>
                      {#if job.nextAttemptAt && job.status === JobStatus.QUEUED}
                        <div>
                          <dt class="text-sm text-muted">{m('admin.jobs.next_attempt')}</dt>
                          <dd>{timestamp(job.nextAttemptAt)}</dd>
                        </div>
                      {/if}
                      {#if job.failure}
                        <div class="min-w-0 sm:col-span-2">
                          <dt class="text-sm text-muted">{m('common.reason')}</dt>
                          <dd class="font-mono text-sm break-words">{job.failure}</dd>
                        </div>
                      {/if}
                    </dl>
                    {#if job.status === JobStatus.FAILED}
                      <div class="flex flex-wrap items-center gap-3">
                        <Button
                          variant="secondary"
                          onclick={() => retry(job)}
                          disabled={retryMutation.isPending ||
                            job.retryState !== RetryJobState.ALLOWED}
                          loading={retryMutation.isPending &&
                            retryMutation.variables?.id === job.id}>{m('common.retry')}</Button
                        >
                        {#if job.retryState === RetryJobState.EXPIRED}
                          <span class="text-sm text-muted">{m('admin.jobs.expired')}</span>
                        {:else if job.retryState !== RetryJobState.ALLOWED}
                          <span class="text-sm text-muted">{m('common.action_unavailable')}</span>
                        {/if}
                      </div>
                    {/if}
                  </div>
                </details>
              </li>
            {/each}
          </ul>
          {#if jobsQuery.hasNextPage}
            <div class="flex justify-center">
              <Button
                variant="secondary"
                onclick={() => jobsQuery.fetchNextPage()}
                loading={jobsQuery.isFetchingNextPage}
                disabled={jobsQuery.isFetching}
              >
                {m('admin.jobs.load_more')}
              </Button>
            </div>
          {/if}
        {/if}
      </div>
    </Panel>
  </PaneContent>
</div>
