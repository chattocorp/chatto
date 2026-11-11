<!-- @component Safe details of one background job, with retry for an eligible failed job. -->
<script lang="ts">
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { createAdminJobsAPI, JobStatus, RetryJobState } from '$lib/api/adminJobs';
  import { m } from '$lib/i18n/messages';
  import { getLocale } from '$lib/i18n/runtime';
  import { serverIdToSegment } from '$lib/navigation';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { adminQueryKeys } from '$lib/query/admin';
  import { createQuery } from '$lib/query/client';
  import { Hint, LoadingFog, PageTitle, PaneContent, PaneHeader, Panel } from '$lib/ui';
  import { Button } from '$lib/ui/form';
  import { errorMessage } from '$lib/utils/errorMessage';
  import { formatDateTime, timeFormatSettingsFor } from '$lib/utils/formatTime';
  import { JOBS_POLL_INTERVAL_MS, jobTypeLabel } from '../jobPresentation';
  import JobStatusPill from '../JobStatusPill.svelte';
  import { canRetryJob, createJobRetry } from '../jobRetry.svelte';

  const scope = useServerScope();
  const connection = scope.connection;
  const api = connection.getAPI(createAdminJobsAPI);
  const timeSettings = $derived(timeFormatSettingsFor(scope.store.currentUser.user?.settings));
  const locale = $derived(getLocale());

  const id = $derived(page.params.id!);
  const queryKey = $derived(adminQueryKeys.job(scope.serverId, connection, id));
  const jobQuery = createQuery(() => ({
    queryKey,
    queryFn: ({ signal }) => api.getJob(id, { signal }),
    refetchOnMount: 'always' as const,
    // Poll only while the job can still change on its own.
    refetchInterval: (query) =>
      query.state.data?.status === JobStatus.QUEUED ||
      query.state.data?.status === JobStatus.RUNNING
        ? JOBS_POLL_INTERVAL_MS
        : false
  }));
  const backHref = $derived(
    resolve('/chat/[serverId]/manage/server/jobs', { serverId: serverIdToSegment(scope.serverId) })
  );

  const retry = createJobRetry(scope);

  function timestamp(value: Date | null): string {
    return value ? formatDateTime(value.toISOString(), timeSettings, locale) : '—';
  }
</script>

<PageTitle
  title={m('admin.common.page_title', {
    title: jobQuery.data ? jobTypeLabel(jobQuery.data) : m('admin.jobs.title')
  })}
/>

<div class="pane-page">
  <PaneHeader
    title={jobQuery.data ? jobTypeLabel(jobQuery.data) : m('admin.jobs.title')}
    subtitle={m('admin.jobs.detail_subtitle')}
    {backHref}
  />

  <PaneContent>
    <div class="flex min-h-0 flex-col gap-6">
      {#if jobQuery.isPending}
        <LoadingFog class="h-40 w-full" label={m('common.loading')} />
      {:else if jobQuery.error}
        <Hint tone="danger">{errorMessage(jobQuery.error, m('admin.jobs.load_failed'))}</Hint>
      {:else if !jobQuery.data}
        <Hint tone="warning">{m('admin.jobs.not_found')}</Hint>
      {:else}
        {@const job = jobQuery.data}
        <Panel title={m('admin.jobs.details')}>
          {#snippet actions()}
            {#if job.status === JobStatus.FAILED}
              <Button
                size="sm"
                variant="secondary"
                onclick={() => retry.retry(job)}
                disabled={retry.pending || !canRetryJob(job)}
                loading={retry.retrying(job)}
              >
                <span aria-hidden="true" class="iconify icon-[uil--redo] rtl:-scale-x-100"></span>
                {m('common.retry')}
              </Button>
            {/if}
          {/snippet}
          <div class="flex flex-col gap-4">
            {#if job.status === JobStatus.FAILED && job.retryState === RetryJobState.EXPIRED}
              <Hint tone="warning">{m('admin.jobs.expired')}</Hint>
            {:else if job.status === JobStatus.FAILED && !canRetryJob(job)}
              <Hint tone="warning">{m('common.action_unavailable')}</Hint>
            {/if}
            <dl class="grid grid-cols-1 gap-3 sm:grid-cols-[max-content_1fr] sm:gap-x-6">
              <dt class="text-sm text-muted">{m('admin.common.status')}</dt>
              <dd><JobStatusPill status={job.status} /></dd>

              <dt class="text-sm text-muted">{m('admin.common.id')}</dt>
              <dd class="font-mono text-sm break-all">{job.id}</dd>

              <dt class="text-sm text-muted">{m('admin.jobs.attempts')}</dt>
              <dd class="text-sm tabular-nums">
                {job.attempts.toLocaleString(locale)} / {job.maxAttempts.toLocaleString(locale)}
              </dd>

              <dt class="text-sm text-muted">{m('admin.jobs.created')}</dt>
              <dd class="text-sm">{timestamp(job.createdAt)}</dd>

              <dt class="text-sm text-muted">{m('admin.jobs.updated')}</dt>
              <dd class="text-sm">{timestamp(job.updatedAt)}</dd>

              {#if job.nextAttemptAt && job.status === JobStatus.QUEUED}
                <dt class="text-sm text-muted">{m('admin.jobs.next_attempt')}</dt>
                <dd class="text-sm">{timestamp(job.nextAttemptAt)}</dd>
              {/if}

              <dt class="text-sm text-muted">{m('common.reason')}</dt>
              <dd class="font-mono text-sm break-words">{job.failure || '—'}</dd>
            </dl>
          </div>
        </Panel>
      {/if}
    </div>
  </PaneContent>
</div>
