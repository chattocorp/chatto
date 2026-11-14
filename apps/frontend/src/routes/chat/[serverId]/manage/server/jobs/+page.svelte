<!-- @component Server job queue. Each row links to its job details. Failed rows have retry and delete actions. The status filter is in the `status` search parameter. -->
<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { createAdminJobsAPI, JobStatus, type AdminJob } from '$lib/api/adminJobs';
  import FeatureCompatibilityNotice from '$lib/components/chat/FeatureCompatibilityNotice.svelte';
  import { m } from '$lib/i18n/messages';
  import { getLocale } from '$lib/i18n/runtime';
  import { serverIdToSegment } from '$lib/navigation';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { adminQueryKeys } from '$lib/query/admin';
  import { createInfiniteQuery } from '$lib/query/client';
  import { flattenUniquePages } from '$lib/query/pagination';
  import {
    ConfirmDialog,
    DataTable,
    Hint,
    PageTitle,
    PaneContent,
    PaneHeader,
    Panel,
    SegmentedControl,
    ToggleChip
  } from '$lib/ui';
  import { errorMessage } from '$lib/utils/errorMessage';
  import { formatDateTime, timeFormatSettingsFor } from '$lib/utils/formatTime';
  import { JOBS_POLL_INTERVAL_MS, jobAttemptsLabel, jobTypeLabel } from './jobPresentation';
  import { canDeleteJob, canRetryJob, createJobActions } from './jobActions.svelte';
  import JobStatusPill from './JobStatusPill.svelte';

  const scope = useServerScope();
  const connection = scope.connection;
  const api = connection.getAPI(createAdminJobsAPI);
  const timeSettings = $derived(timeFormatSettingsFor(scope.store.currentUser.user?.settings));
  const locale = $derived(getLocale());
  const jobActions = createJobActions(scope);
  /** The failed job that waits for the operator to confirm its deletion. */
  let jobToDelete = $state<AdminJob | null>(null);
  let scrollContainer = $state<HTMLDivElement>();

  const serverSegment = serverIdToSegment(scope.serverId);
  // Servers before the job queue have no AdminJobService. Unknown versions may try.
  const unavailable = $derived(scope.store.serverInfo.limitedFeatures.includes('backgroundJobs'));

  /** Values of the `status` search parameter. Other values select all jobs. */
  const statusFilters = { failed: JobStatus.FAILED, queued: JobStatus.QUEUED } as const;
  type StatusFilter = keyof typeof statusFilters | 'all';
  const statusOptions: { value: StatusFilter; label: string }[] = [
    { value: 'all', label: m('admin.jobs.all') },
    { value: 'failed', label: m('admin.jobs.failed') },
    { value: 'queued', label: m('admin.jobs.queued') }
  ];
  // The URL keeps the filter, so Back from a job restores it.
  const filter = $derived(statusFilterFromUrl(page.url));
  const status = $derived(filter === 'all' ? JobStatus.UNSPECIFIED : statusFilters[filter]);

  function statusFilterFromUrl(url: URL): StatusFilter {
    const value = url.searchParams.get('status');
    return value !== null && Object.hasOwn(statusFilters, value)
      ? (value as keyof typeof statusFilters)
      : 'all';
  }

  function selectStatusFilter(next: StatusFilter) {
    void goto(
      resolve(
        next === 'all'
          ? '/chat/[serverId]/manage/server/jobs'
          : `/chat/[serverId]/manage/server/jobs?status=${next}`,
        { serverId: serverSegment }
      ),
      { replaceState: true, keepFocus: true, noScroll: true }
    );
  }

  // The server lists jobs newest first and filters by status from memory, so
  // polling every loaded page reads no storage.
  const jobsQuery = createInfiniteQuery(() => ({
    queryKey: adminQueryKeys.jobList(scope, status),
    queryFn: ({ pageParam, signal }) => api.listJobs({ cursor: pageParam, status }, { signal }),
    initialPageParam: '',
    getNextPageParam: (lastPage) => lastPage.nextCursor || undefined,
    enabled: !unavailable,
    refetchOnMount: 'always' as const,
    // Poll while the page is visible. TanStack pauses polling in hidden tabs.
    refetchInterval: JOBS_POLL_INTERVAL_MS
  }));

  // Pages may overlap while running jobs change. Keep one row for each stable ID.
  const jobs = $derived(
    flattenUniquePages(
      jobsQuery.data,
      (jobPage) => jobPage.jobs,
      (job) => job.id
    )
  );
  const loadError = $derived(
    jobsQuery.error ? errorMessage(jobsQuery.error, m('admin.jobs.load_failed')) : null
  );

  function jobHref(job: AdminJob) {
    return resolve('/chat/[serverId]/manage/server/jobs/[id]', {
      serverId: serverSegment,
      id: job.id
    });
  }

  function confirmDelete() {
    if (jobToDelete) jobActions.delete(jobToDelete);
    jobToDelete = null;
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
      {#if unavailable}
        <FeatureCompatibilityNotice features={['backgroundJobs']} {serverSegment} />
      {:else}
        <!-- With no jobs loaded, the table shows the error in place of "no jobs". -->
        {#if loadError && jobs.length > 0}
          <Hint tone="danger">{loadError}</Hint>
        {/if}

        <Panel title={m('admin.jobs.title')} noPadding>
          {#snippet actions()}
            <SegmentedControl
              label={m('admin.common.status')}
              options={statusOptions}
              value={filter}
              onchange={selectStatusFilter}
            />
          {/snippet}
          <DataTable
            items={jobs}
            columns={6}
            loading={jobsQuery.isPending}
            emptyMessage={loadError ?? m('admin.jobs.empty')}
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
                {jobAttemptsLabel(job, locale)}
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
                      disabled={jobActions.pending || !canRetryJob(job)}
                      onclick={() => jobActions.retry(job)}
                    >
                      <span
                        class="iconify icon-[uil--redo] text-base rtl:-scale-x-100"
                        role="img"
                        aria-label={m('common.retry')}
                      ></span>
                    </ToggleChip>
                    <ToggleChip
                      tone="neutral"
                      square
                      title={m('common.delete')}
                      disabled={jobActions.pending || !canDeleteJob(job)}
                      onclick={() => (jobToDelete = job)}
                    >
                      <span
                        class="iconify icon-[uil--trash-alt] text-base"
                        role="img"
                        aria-label={m('common.delete')}
                      ></span>
                    </ToggleChip>
                  </div>
                {/if}
              </td>
            {/snippet}
          </DataTable>
        </Panel>
      {/if}
    </div>
  </PaneContent>
</div>

{#if jobToDelete}
  <ConfirmDialog
    title={m('admin.jobs.delete_title')}
    actionLabel={m('common.delete')}
    actionIcon="iconify icon-[uil--trash-alt]"
    onconfirm={confirmDelete}
    onclose={() => (jobToDelete = null)}
  >
    {m('admin.jobs.delete_message')}
  </ConfirmDialog>
{/if}
