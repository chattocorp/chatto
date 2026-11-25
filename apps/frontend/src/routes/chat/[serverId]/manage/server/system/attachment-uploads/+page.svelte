<!-- @component Monthly attachment upload totals and room and uploader breakdowns. -->
<script lang="ts">
  import {
    getAdminAttachmentUploads,
    type AdminAttachmentUploads
  } from '$lib/api/adminDiagnostics';
  import { m } from '$lib/i18n/messages';
  import { getReactiveLocale } from '$lib/i18n/state.svelte';
  import { adminQueryKeys } from '$lib/query/admin';
  import { createQuery } from '$lib/query/client';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { DataTable, EmptyState, Hint, LoadingDots, Panel } from '$lib/ui';
  import { Button, FormField } from '$lib/ui/form';
  import { errorMessage } from '$lib/utils/errorMessage';
  import { useSystem } from '../systemContext';

  const system = useSystem();
  const scope = useServerScope();
  const monthInputId = $props.id();

  // The API uses UTC, including when the browser is already in the next month.
  const currentMonth = new Date().toISOString().slice(0, 7);
  let month = $state(currentMonth);
  let draftMonth = $state(currentMonth);

  const supported = $derived(system.info.attachmentUploadsAvailable);
  const report = createQuery(() => {
    const requestedMonth = month;
    return {
      enabled: supported,
      queryKey: adminQueryKeys.attachmentUploadsMonth(scope, requestedMonth),
      queryFn: ({ signal }) =>
        getAdminAttachmentUploads(scope.connection.apiConfig, requestedMonth, { signal })
    };
  });

  const locale = $derived(getReactiveLocale());
  const numberFormat = $derived(new Intl.NumberFormat(locale));
  const monthLabel = $derived(
    new Intl.DateTimeFormat(locale, {
      month: 'long',
      year: 'numeric',
      timeZone: 'UTC',
      calendar: 'gregory'
    }).format(new Date(`${month}-01T00:00:00Z`))
  );

  function selectMonth(event: SubmitEvent) {
    event.preventDefault();
    month = draftMonth;
  }
</script>

{#if !supported}
  <Hint>{m('admin.system.attachment_uploads.unavailable')}</Hint>
{:else}
  <Panel title={m('admin.system.attachment_uploads.title')} icon="iconify icon-[uil--upload]">
    <div class="flex flex-col gap-4">
      <p class="text-muted">{m('admin.system.attachment_uploads.description')}</p>

      <form class="flex flex-wrap items-end gap-2" onsubmit={selectMonth}>
        <FormField id={monthInputId} label={m('admin.system.attachment_uploads.month')}>
          <!-- Browsers without a month picker use a text input with the same wire format. -->
          <input
            id={monthInputId}
            class="input"
            type="month"
            required
            max="9999-12"
            pattern="[0-9]{4}-(0[1-9]|1[0-2])"
            placeholder="YYYY-MM"
            bind:value={draftMonth}
          />
        </FormField>
        <Button type="submit" variant="neutral" disabled={draftMonth === month}>
          {m('admin.system.attachment_uploads.show')}
        </Button>
      </form>

      {#if report.isPending}
        <LoadingDots class="h-32" label={m('admin.system.attachment_uploads.loading')} />
      {:else if report.error}
        <Hint tone="danger">
          {errorMessage(report.error, m('admin.system.attachment_uploads.error'))}
        </Hint>
      {:else if report.data === null}
        <Hint>{m('admin.system.attachment_uploads.unavailable')}</Hint>
      {:else if report.data}
        <h3 class="font-semibold"><time datetime={month}>{monthLabel}</time></h3>
        <dl class="flex flex-wrap gap-x-8 gap-y-2 tabular-nums">
          <div>
            <dt class="text-muted">{m('admin.system.attachment_uploads.total_files')}</dt>
            <dd class="text-lg font-semibold">{numberFormat.format(report.data.total.files)}</dd>
          </div>
          <div>
            <dt class="text-muted">{m('admin.system.attachment_uploads.total_bytes')}</dt>
            <dd class="text-lg font-semibold">{numberFormat.format(report.data.total.bytes)}</dd>
          </div>
        </dl>
        {#if report.data.total.files === 0n}
          <EmptyState title={m('admin.system.attachment_uploads.empty')} />
        {/if}
      {/if}
    </div>
  </Panel>

  {#if !report.error && report.data && report.data.total.files > 0n}
    {@render breakdown(
      m('admin.system.attachment_uploads.by_room'),
      m('admin.common.room'),
      m('admin.system.attachment_uploads.private_rooms'),
      report.data.rooms
    )}
    {@render breakdown(
      m('admin.system.attachment_uploads.by_user'),
      m('admin.common.user'),
      m('admin.system.attachment_uploads.operator_imports'),
      report.data.users
    )}
  {/if}
{/if}

{#snippet breakdown(
  title: string,
  nameLabel: string,
  combinedLabel: string,
  rows: AdminAttachmentUploads['rooms']
)}
  <Panel {title} noPadding>
    <DataTable items={rows} columns={3} stickyHeader hoverable={false}>
      {#snippet header()}
        <th scope="col" class="px-4 py-2 text-start">{nameLabel}</th>
        <th scope="col" class="px-4 py-2 text-end">{m('admin.system.bytes')}</th>
        <th scope="col" class="px-4 py-2 text-end">{m('admin.system.attachment_uploads.files')}</th>
      {/snippet}
      {#snippet row(row)}
        <th scope="row" class="w-full px-4 py-2 text-start font-normal wrap-anywhere">
          <bdi>{row.id ? row.name || row.id : combinedLabel}</bdi>
        </th>
        <td class="px-4 py-2 text-end whitespace-nowrap tabular-nums">
          {numberFormat.format(row.totals.bytes)}
        </td>
        <td class="px-4 py-2 text-end whitespace-nowrap tabular-nums">
          {numberFormat.format(row.totals.files)}
        </td>
      {/snippet}
    </DataTable>
  </Panel>
{/snippet}
