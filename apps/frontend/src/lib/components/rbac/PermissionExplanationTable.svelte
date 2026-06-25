<script lang="ts">
  import { SvelteSet } from 'svelte/reactivity';
  import { Pill } from '$lib/ui';
  import { getPermissionDescription } from '$lib/permissions';
  import * as m from '$lib/i18n/messages';

  type DecisionKind = 'ALLOW' | 'DENY' | 'NONE';
  type Level = 'SERVER' | 'GROUP' | 'ROOM';

  type TraceEntry = {
    level: Level;
    roleName: string;
    decision: DecisionKind;
    applied: boolean;
  };

  type Explanation = {
    permission: string;
    state: DecisionKind;
    decidedAt?: Level | null;
    decidedByRole?: string | null;
    trace: TraceEntry[];
  };

  let { explanations }: { explanations: Explanation[] } = $props();

  const expanded = new SvelteSet<string>();

  function toggle(permission: string) {
    if (expanded.has(permission)) {
      expanded.delete(permission);
    } else {
      expanded.add(permission);
    }
  }

  function levelLabel(level: Level): string {
    switch (level) {
      case 'SERVER':
        return m['rbac.permissions.level_server']();
      case 'GROUP':
        return m['rbac.permissions.level_group']();
      case 'ROOM':
        return m['rbac.permissions.level_room']();
    }
  }
</script>

<div class="grid grid-cols-[1fr_auto_minmax(12rem,1.5fr)_auto] items-center gap-x-4 text-sm">
  <div class="border-b border-border pb-2 font-medium text-muted">
    {m['rbac.permissions.permission']()}
  </div>
  <div class="border-b border-border pb-2 text-center font-medium text-muted">
    {m['rbac.permissions.state']()}
  </div>
  <div class="border-b border-border pb-2 font-medium text-muted">
    {m['rbac.permissions.decided_by']()}
  </div>
  <div class="border-b border-border pb-2"></div>

  {#each explanations as exp (exp.permission)}
    {@const isExpanded = expanded.has(exp.permission)}
    {@const hasTrace = exp.trace.length > 0}

    <div class="flex flex-col border-b border-border/50 py-2">
      <code class="text-sm font-medium">{exp.permission}</code>
      <div class="text-xs text-muted">{getPermissionDescription(exp.permission)}</div>
    </div>

    <div class="flex items-center justify-center border-b border-border/50 py-2">
      {#if exp.state === 'ALLOW'}
        <span
          class="iconify text-lg text-success uil--check-circle"
          title={m['rbac.permissions.granted']()}
        ></span>
      {:else if exp.state === 'DENY'}
        <span
          class="iconify text-lg text-danger uil--times-circle"
          title={m['rbac.permissions.denied']()}
        ></span>
      {:else}
        <span
          class="iconify text-lg text-muted uil--minus-circle"
          title={m['rbac.permissions.no_decision']()}
        ></span>
      {/if}
    </div>

    <div class="flex items-center gap-2 border-b border-border/50 py-2 text-xs">
      {#if exp.state === 'NONE' || !exp.decidedAt}
        <span class="text-muted italic">{m['rbac.permissions.no_role_decided']()}</span>
      {:else}
        <Pill tone="muted">{levelLabel(exp.decidedAt)}</Pill>
        <span class="font-medium">{exp.decidedByRole}</span>
      {/if}
    </div>

    <div class="flex items-center border-b border-border/50 py-2 pl-2">
      {#if hasTrace}
        <button
          type="button"
          onclick={() => toggle(exp.permission)}
          aria-expanded={isExpanded}
          class="cursor-pointer rounded px-1 text-muted hover:bg-surface-100"
          title={isExpanded
            ? m['rbac.permissions.hide_trace']()
            : m['rbac.permissions.show_trace']()}
        >
          <span
            class={[
              'iconify text-lg transition-transform',
              isExpanded ? 'uil--angle-down' : 'uil--angle-right'
            ]}
          ></span>
        </button>
      {/if}
    </div>

    {#if isExpanded}
      <div class="col-span-4 border-b border-border/50 bg-surface-100/70 px-4 py-3 text-xs">
        <div class="mb-2 font-medium text-muted">
          {exp.trace.length === 1
            ? m['rbac.permissions.trace_one']()
            : m['rbac.permissions.trace_many']({ count: exp.trace.length })}
        </div>
        <ol class="flex flex-col gap-1">
          {#each exp.trace as entry, i (i)}
            <li class="flex items-center gap-2">
              <Pill tone="muted">{levelLabel(entry.level)}</Pill>
              <span class="font-medium">{entry.roleName}</span>
              <Pill tone={entry.decision === 'ALLOW' ? 'success' : 'danger'}>
                {entry.decision === 'ALLOW'
                  ? m['rbac.permissions.allow']()
                  : m['rbac.permissions.deny']()}
              </Pill>
              {#if entry.applied}
                <span class="text-muted italic">{m['rbac.permissions.winning_decision']()}</span>
              {/if}
            </li>
          {/each}
        </ol>
      </div>
    {/if}
  {/each}
</div>
