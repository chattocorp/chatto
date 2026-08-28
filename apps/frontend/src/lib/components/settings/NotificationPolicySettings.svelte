<!--
@component

User notification preferences across server, room-group, and room scopes.
Rows are notification causes. Columns follow the current navigation layout.
-->
<script lang="ts">
  import { Panel } from '$lib/components/admin';
  import { MatrixCellButton, MatrixTable } from '$lib/components/matrix';
  import { HelpTooltip, Hint } from '$lib/ui';
  import { ShortcutTextInput } from '$lib/ui/form';
  import { m } from '$lib/i18n/messages';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import {
    NotificationDeliveryMode,
    type NotificationPolicyField
  } from '$lib/api-client/notifications';
  import NotificationPolicyCell from './NotificationPolicyCell.svelte';
  import {
    notificationPolicyCellApplicable,
    notificationPolicyColumns,
    type NotificationPolicyColumn
  } from './notificationPolicyMatrix';
  import {
    notificationDeliveryModeLabel,
    notificationDeliveryModePresentation
  } from './notificationDeliveryModePresentation';

  type NotificationPolicyRow = {
    field: NotificationPolicyField;
    label: string;
    hint: string;
  };

  const serverScope = useServerScope();
  const notificationStore = $derived(serverScope.store.notifications);
  const matrixState = $derived(notificationStore.notificationPolicies);
  let scopeFilter = $state('');

  const rows = $derived<NotificationPolicyRow[]>([
    {
      field: 'directMessages',
      label: m('settings.notifications.policy.reason.direct_message'),
      hint: m('settings.notifications.policy.reason_hint.direct_message')
    },
    {
      field: 'roomMessages',
      label: m('settings.notifications.policy.reason.room_message'),
      hint: m('settings.notifications.policy.reason_hint.room_message')
    },
    {
      field: 'directMentions',
      label: m('settings.notifications.policy.reason.direct_mention'),
      hint: m('settings.notifications.policy.reason_hint.direct_mention')
    },
    {
      field: 'replies',
      label: m('settings.notifications.policy.reason.reply'),
      hint: m('settings.notifications.policy.reason_hint.reply')
    },
    {
      field: 'roleMentions',
      label: m('settings.notifications.policy.reason.role_mention'),
      hint: m('settings.notifications.policy.reason_hint.role_mention')
    },
    {
      field: 'hereMentions',
      label: m('settings.notifications.policy.reason.here'),
      hint: m('settings.notifications.policy.reason_hint.here')
    },
    {
      field: 'allMentions',
      label: m('settings.notifications.policy.reason.all'),
      hint: m('settings.notifications.policy.reason_hint.all')
    },
    {
      field: 'followedThreads',
      label: m('settings.notifications.policy.reason.followed_thread'),
      hint: m('settings.notifications.policy.reason_hint.followed_thread')
    },
    {
      field: 'reactions',
      label: m('settings.notifications.policy.reason.reaction'),
      hint: m('settings.notifications.policy.reason_hint.reaction')
    }
  ]);

  const columns = $derived(
    notificationPolicyColumns(
      serverScope.store.serverInfo.name,
      serverScope.store.navigation.roomGroups,
      serverScope.store.navigation.rooms,
      scopeFilter
    )
  );
  $effect(() => {
    void matrixState.load(columns.map((item) => item.scope));
  });

  function columnClass(column: NotificationPolicyColumn): string {
    if (column.kind === 'server') return 'bg-surface-emphasized/40';
    if (column.kind === 'roomGroup') return 'bg-surface-emphasized/20';
    return '';
  }

  function notApplicableLabel(row: NotificationPolicyRow, column: NotificationPolicyColumn) {
    return m('settings.notifications.policy.not_applicable_cell_aria', {
      cause: row.label,
      scope: column.label
    });
  }
</script>

<Panel
  title={m('settings.notifications.policy.title')}
  subtitle={m('settings.notifications.policy.description')}
  noPadding
>
  {#snippet actions()}
    <div class="w-48 sm:w-64">
      <ShortcutTextInput
        id="notification-scope-filter"
        testid="notification-scope-filter"
        label={m('settings.notifications.policy.filter_label')}
        labelHidden
        shortcutKey="/"
        placeholder={m('settings.notifications.policy.filter_placeholder')}
        leadingIcon="iconify icon-[uil--search]"
        autocomplete="off"
        bind:value={scopeFilter}
      />
    </div>
  {/snippet}

  <div
    class="flex flex-wrap items-center gap-x-4 gap-y-2 border-b border-border bg-surface/50 px-4 py-2 text-xs text-muted"
    aria-label={m('settings.notifications.policy.legend')}
  >
    {#each [NotificationDeliveryMode.OFF, NotificationDeliveryMode.UNREAD_BADGE, NotificationDeliveryMode.IN_APP_NOTIFICATION, NotificationDeliveryMode.PUSH_NOTIFICATION] as mode (mode)}
      {@const presentation = notificationDeliveryModePresentation(mode)}
      <span class="inline-flex items-center gap-1.5">
        <span
          class={['iconify h-5 w-5 shrink-0', presentation.icon, presentation.legendClass]}
          aria-hidden="true"
        ></span>
        {notificationDeliveryModeLabel(mode)}
        {#if mode === NotificationDeliveryMode.UNREAD_BADGE}
          <HelpTooltip
            label={`${m('ui.tooltip.more_information')}: ${notificationDeliveryModeLabel(mode)}`}
          >
            {m('settings.notifications.policy.delivery_mode.badge_hint')}
          </HelpTooltip>
        {/if}
      </span>
    {/each}
  </div>

  {#if matrixState.error}
    <div class="px-4 pt-3">
      <Hint tone="danger">
        {matrixState.errorKind === 'save'
          ? m('settings.notifications.policy.save_failed')
          : m('settings.notifications.policy.load_failed')}: {matrixState.error}
      </Hint>
    </div>
  {/if}

  <MatrixTable
    {rows}
    {columns}
    getRowKey={(row) => row.field}
    getColumnKey={(column) => column.key}
    emptyMessage={m('settings.notifications.policy.no_filter_matches')}
    {columnClass}
    columnAttributes={(column) => ({ 'data-notification-scope': column.key })}
    cellAttributes={(row, column) => ({
      'data-notification-scope': column.key,
      'data-notification-field': row.field
    })}
    isCellInteractive={(row, column) =>
      notificationPolicyCellApplicable(row.field, column) &&
      Boolean(matrixState.policy(column.scope)) &&
      !matrixState.isPending(column.scope, row.field)}
    spacerTestId="notification-matrix-spacer"
  >
    {#snippet leadingHeader()}
      {m('settings.notifications.policy.reason.activity')}
    {/snippet}
    {#snippet columnHeader(column, highlighted)}
      <span
        class={[
          column.kind === 'server' ? 'font-semibold' : '',
          highlighted
            ? 'text-action'
            : column.kind === 'roomGroup'
              ? 'text-neutral-action'
              : 'text-muted'
        ]}
        title={column.label}
      >
        {column.displayLabel}
      </span>
    {/snippet}
    {#snippet rowHeader(row, highlighted)}
      <div class="flex items-center gap-2">
        <HelpTooltip label={`${m('ui.tooltip.more_information')}: ${row.label}`}>
          {row.hint}
        </HelpTooltip>
        <span class={['text-sm', highlighted ? 'text-action' : '']}>{row.label}</span>
      </div>
    {/snippet}
    {#snippet cell(row, column)}
      {@const applicable = notificationPolicyCellApplicable(row.field, column)}
      {#if !applicable}
        {@const label = notApplicableLabel(row, column)}
        <MatrixCellButton
          applicable={false}
          icon=""
          ariaLabel={label}
          onActivate={() => undefined}
        />
      {:else if matrixState.policy(column.scope)}
        {@const policy = matrixState.policy(column.scope)!}
        <NotificationPolicyCell
          field={row.field}
          causeLabel={row.label}
          scope={column.scope}
          scopeLabel={column.label}
          override={policy.overrides[row.field]}
          effective={policy.effective[row.field]}
          loading={matrixState.isPending(column.scope, row.field)}
          onChange={(next) => void matrixState.update(column.scope, row.field, next)}
        />
      {:else}
        <span
          class="inline-flex h-10 w-10 items-center justify-center"
          role={matrixState.loading ? 'status' : undefined}
        >
          {#if matrixState.loading}
            <span class="iconify icon-[uil--spinner] animate-spin text-muted" aria-hidden="true"
            ></span>
            <span class="sr-only">{m('common.loading')}</span>
          {:else}
            <span class="text-muted/30" aria-hidden="true">—</span>
          {/if}
        </span>
      {/if}
    {/snippet}
  </MatrixTable>
</Panel>
