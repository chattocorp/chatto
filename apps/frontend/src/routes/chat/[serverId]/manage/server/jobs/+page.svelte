<!-- @component Server job queue. Each row links to its job details and retry action. -->
<script lang="ts">
  import { resolve } from '$app/paths';
  import { SvelteMap } from 'svelte/reactivity';
  import { createAdminJobsAPI, JobStatus, type AdminJob } from '$lib/api/adminJobs';
  import { m } from '$lib/i18n/messages';
  import { getLocale } from '$lib/i18n/runtime';
  import { serverIdToSegment } from '$lib/navigation';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { adminQueryKeys } from '$lib/query/admin';
  import { createInfiniteQuery } from '$lib/query/client';
  import { DataTable, Hint, PageTitle, PaneContent, PaneHeader, Panel, ToggleChip } from '$lib/ui';
  import { Button } from '$lib/ui/form';
  import { errorMessage } from '$lib/utils/errorMessage';
  import { formatDateTime, timeFormatSettingsFor } from '$lib/utils/formatTime';
  import { JOBS_POLL_INTERVAL_MS, jobTypeLabel } from './jobPresentation';
  import { canRetryJob, createJobRetry } from './jobRetry.svelte';
  import JobStatusPill from './JobStatusPill.svelte';

  const scope = useServerScope();
  const connection = scope.connection;
  const api = connection.getAPI(createAdminJobsAPI);
  const timeSettings = $derived(timeFormatSettingsFor(scope.store.currentUser.user?.settings));
  const locale = $derived(getLocale());
  const retry = createJobRetry(scope);
  let scrollContainer = $state<HTMLDivElement>();

  const jobsQuery = createInfiniteQuery(() => ({
    queryKey: adminQueryKeys.jobs(scope.serverId, connection),
    queryFn: ({ pageParam, signal }) => api.listJobs({ pageToken: pageParam }, { signal }),
    initialPageParam: '',
    getNextPageParam: (lastPage) => lastPage.nextPageToken || undefined,
    refetchOnMount: 'always' as const,
    // Poll while the page is visible. TanStack pauses polling in hidden tabs.
    refetchInterval: JOBS_POLL_INTERVAL_MS
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

  function jobHref(job: AdminJob) {
    return resolve('/chat/[serverId]/manage/server/jobs/[id]', {
      serverId: serverIdToSegment(scope.serverId),
      id: job.id
    });
  }

  function timestamp(value: Date | null): string {
    return value ? formatDateTime(value.toISOString(), timeSettings, locale) : '—';
  }
</script>

<PageTitle title={m('admin.common.page_title', { title: m('admin.jobs.title') })} />

<div class="pane-page">
  <PaneHeader title={m('admin.jobs.title')} subtitle={m('admin.jobs.subtitle')} />

  <PaneContent bind:scrollContainer>
    <div class="flex flex-col gap-6">
      {#if loadError}
        <Hint tone="danger">{loadError}</Hint>
      {/if}

      <Panel title={m('admin.jobs.title')} noPadding>
        {#snippet actions()}
          <Button
            size="sm"
            variant="secondary"
            onclick={() => jobsQuery.refetch()}
            disabled={jobsQuery.isFetching}
          >
            <span aria-hidden="true" class="iconify icon-[uil--sync]"></span>
            {m('admin.system.refresh')}
          </Button>
        {/snippet}
        <DataTable
          items={jobs}
          columns={6}
          loading={jobsQuery.isPending}
          emptyMessage={m('admin.jobs.empty')}
          getKey={(job) => job.id}
          hasMore={jobsQuery.hasNextPage && !loadError}
          loadingMore={jobsQuery.isFetchingNextPage}
          onLoadMore={async () => {
            await jobsQuery.fetchNextPage();
          }}
          loadMoreRoot={scrollContainer}
          loadingMoreMessage={m('common.loading')}
        >
          {#snippet header()}
            <th class="table-header-cell">{m('admin.jobs.job')}</th>
            <th class="table-header-cell">{m('admin.common.status')}</th>
            <th class="table-header-cell">{m('admin.jobs.attempts')}</th>
            <th class="table-header-cell">{m('admin.jobs.updated')}</th>
            <th class="table-header-cell">{m('common.reason')}</th>
            <th class="table-header-cell"
              ><span class="sr-only">{m('admin.neighbors.actions')}</span></th
            >
          {/snippet}
          {#snippet row(job)}
            <td class="px-4 py-3 text-sm font-medium">
              <a class="data-table-row-link" href={jobHref(job)}>{jobTypeLabel(job)}</a>
            </td>
            <td class="px-4 py-3">
              <JobStatusPill status={job.status} />
            </td>
            <td class="px-4 py-3 text-sm tabular-nums">
              {job.attempts.toLocaleString(locale)} / {job.maxAttempts.toLocaleString(locale)}
            </td>
            <td class="px-4 py-3 text-sm">{timestamp(job.updatedAt ?? job.createdAt)}</td>
            <td class="px-4 py-3 font-mono text-xs text-muted">{job.failure || '—'}</td>
            <td class="px-3 py-2">
              {#if job.status === JobStatus.FAILED}
                <div class="flex items-center justify-end gap-1">
                  <ToggleChip
                    tone="neutral"
                    square
                    title={canRetryJob(job) ? m('common.retry') : m('common.action_unavailable')}
                    disabled={retry.pending || !canRetryJob(job)}
                    onclick={() => retry.retry(job)}
                  >
                    <span
                      class="iconify icon-[uil--redo] text-base rtl:-scale-x-100"
                      role="img"
                      aria-label={m('common.retry')}
                    ></span>
                  </ToggleChip>
                </div>
              {/if}
            </td>
          {/snippet}
        </DataTable>
      </Panel>
    </div>
  </PaneContent>
</div>
