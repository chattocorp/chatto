<!--
@component

Roles page: the server roles in their role order, with a link to create a
role and an edit (or view) link on each row. The role order is an
administrative rank: it decides who can manage whom. It does not change how
permissions resolve.

The list shows the roles in the server's order, highest first. `owner` is
fixed at the top and `everyone` at the bottom. Role managers can drag every
other role, also above their own highest role, and edit every role except
`owner`, which only owners edit (ADR-114).

A pointer drag saves when it is dropped. A keyboard drag reports every arrow
key move as a `finalize` event while the drag continues, so it saves once when
the drag stops (`consider` with the `dragStopped` trigger). A save sends one
relative move: the dragged role and the role that now follows it. It does not
send the full order, so roles that others create or delete at the same time
do not break it.
-->
<script lang="ts">
  import { resolve } from '$app/paths';
  import { flip } from 'svelte/animate';
  import { onMount } from 'svelte';
  import {
    dragHandle,
    dragHandleZone,
    setAriaStrings,
    SOURCES,
    TRIGGERS,
    type DndEvent
  } from 'svelte-dnd-action';
  import { createRoleAPI, type RoleCatalog, type ServerRole } from '@chatto/client/api/roles';
  import { dragAndDropAriaStrings } from '$lib/i18n/dragAndDropAria';
  import { m } from '$lib/i18n/messages';
  import { serverIdToSegment } from '$lib/navigation';
  import { adminQueryKeys } from '$lib/query/admin';
  import { createMutation, createQuery, queryClient, refreshRoleQueries } from '$lib/query/client';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { createSessionGuard, type SessionSnapshot } from '$lib/state/server/sessionGuard.svelte';
  import { Hint, LoadingFog, PageTitle, PaneContent, PaneHeader, Panel, Pill } from '$lib/ui';
  import { Button } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';
  import { errorMessage, toastError } from '$lib/utils/errorMessage';

  type RoleItem = ServerRole & { id: string };
  type MoveVariables = SessionSnapshot & {
    roleName: string;
    /** The movable role directly below the moved role, or none for the lowest place. */
    beforeRoleName?: string;
  };

  const serverScope = useServerScope();
  const session = createSessionGuard(serverScope);
  const serverSegment = serverIdToSegment(serverScope.serverId);
  const canManageRoles = $derived(serverScope.store.permissions.canAdminManageRoles);

  const rolesQuery = createQuery(() => {
    const serverId = serverScope.serverId;
    const connection = serverScope.connection;
    return {
      queryKey: adminQueryKeys.roleCatalog(serverId, connection),
      queryFn: ({ signal }) => connection.getAPI(createRoleAPI).listAdminRoles({ signal }),
      enabled: canManageRoles
    };
  });

  /** Every role, highest first, as the server lists them. */
  const roles = $derived(rolesQuery.data?.roles ?? []);
  const ownerRole = $derived(roles.find((role) => role.name === 'owner') ?? null);
  const everyoneRole = $derived(roles.find((role) => role.name === 'everyone') ?? null);
  /** The saved order of the roles that can move, highest first. */
  const savedMovableItems = $derived(
    roles
      .filter((role) => role.name !== 'owner' && role.name !== 'everyone')
      .map((role) => ({ ...role, id: role.name }))
  );
  // Role managers change every role; only owners change the owner role.
  const roleCatalog = serverScope.store.roleCatalog;
  const editableRoles = $derived(
    new Set(
      roles
        .filter((role) => roleCatalog.canChangeRole(role.name, canManageRoles))
        .map((role) => role.name)
    )
  );

  // The local order while a drag or its save runs. `null` shows the saved order.
  let draftItems = $state.raw<RoleItem[] | null>(null);
  const movableItems = $derived(draftItems ?? savedMovableItems);

  // svelte-dnd-action announces in English by default. The setting is global,
  // so restore the defaults when the page closes.
  onMount(() => {
    setAriaStrings(dragAndDropAriaStrings());
    return () => setAriaStrings(null);
  });

  const moveMutation = createMutation(() => ({
    mutationFn: ({ connection, roleName, beforeRoleName }: MoveVariables) =>
      connection.getAPI(createRoleAPI).moveRole(roleName, beforeRoleName),
    onSuccess: (updatedRoles, variables) => {
      if (!session.isCurrent(variables)) return;
      queryClient.setQueryData<RoleCatalog>(
        adminQueryKeys.roleCatalog(variables.serverId, variables.connection),
        (current) => (current ? { ...current, roles: updatedRoles } : current)
      );
      refreshRoleQueries(variables.serverId);
      toast.success(m('admin.permissions.role_order.saved'));
    },
    onError: (error, variables) => {
      if (session.isCurrent(variables)) {
        toastError(error, m('admin.permissions.role_order.save_failed'));
      }
    },
    onSettled: () => {
      // After a failure, this shows the cached order again.
      draftItems = null;
    }
  }));

  const saving = $derived(moveMutation.isPending && session.isCurrent(moveMutation.variables));

  // svelte-dnd-action changes its items array in place during keyboard drags,
  // so keep a copy to publish each change.
  function handleConsider(event: CustomEvent<DndEvent<RoleItem>>) {
    const { items, info } = event.detail;
    if (info.source === SOURCES.KEYBOARD && info.trigger === TRIGGERS.DRAG_STOPPED) {
      saveMove(items, info.id);
    } else {
      draftItems = [...items];
    }
  }

  function handleFinalize(event: CustomEvent<DndEvent<RoleItem>>) {
    const { items, info } = event.detail;
    if (info.source === SOURCES.KEYBOARD) {
      // An arrow key moved the item. The keyboard drag continues.
      draftItems = [...items];
    } else {
      saveMove(items, info.id);
    }
  }

  /**
   * Saves the move of the dragged role `movedId` after a drag ends, or shows
   * the saved order when nothing moved.
   */
  function saveMove(items: RoleItem[], movedId: string) {
    const movedIndex = items.findIndex((role) => role.id === movedId);
    const unchanged =
      items.length === savedMovableItems.length &&
      savedMovableItems.every((role, index) => items[index]?.name === role.name);
    if (movedIndex < 0 || unchanged) {
      draftItems = null;
      return;
    }
    draftItems = [...items];
    // The moved role ranks directly above the role after it in the list
    // (highest first). At the bottom of the list, it goes lowest.
    moveMutation.mutate({
      ...session.snapshot(),
      roleName: items[movedIndex].name,
      beforeRoleName: items[movedIndex + 1]?.name
    });
  }
</script>

<!--
  Every role opens. Roles at or above the viewer's highest role open
  read-only, so they show a view icon instead of an edit icon. The link uses
  the look of the square icon chips on the room rows.
-->
{#snippet openAction(role: ServerRole)}
  {@const editable = editableRoles.has(role.name)}
  {@const label = editable
    ? m('admin.permissions.roles_page.edit_role', { role: role.displayName })
    : m('admin.permissions.roles_page.view_role', { role: role.displayName })}
  <a
    href={resolve('/chat/[serverId]/manage/server/roles/[name]', {
      serverId: serverSegment,
      name: role.name
    })}
    class="toggle-chip w-10 shrink-0 rounded-lg border-input-border bg-surface p-0 text-muted hover:bg-surface-emphasized hover:text-text"
    title={label}
    aria-label={label}
  >
    <span
      class={['iconify text-base', editable ? 'icon-[uil--pen]' : 'icon-[uil--eye]']}
      aria-hidden="true"
    ></span>
  </a>
{/snippet}

{#snippet roleRow(role: ServerRole, badge: string | null)}
  <div class="flex min-w-0 flex-1 items-center gap-2">
    <span class="min-w-0 truncate font-medium" dir="auto">{role.displayName}</span>
    <span class="min-w-0 truncate text-muted">@{role.name}</span>
  </div>
  {#if badge}
    <Pill tone="muted" class="shrink-0">{badge}</Pill>
  {/if}
  {@render openAction(role)}
{/snippet}

{#snippet fixedRow(role: ServerRole, badge: string)}
  <div class="flex items-center gap-3 py-2 ps-3 pe-4" data-role={role.name} data-locked>
    <span class="iconify icon-[uil--lock] shrink-0 text-lg text-muted" aria-hidden="true"></span>
    {@render roleRow(role, badge)}
  </div>
{/snippet}

<PageTitle
  title={m('admin.common.server_admin_page_title', {
    title: m('admin.permissions.roles_page.title')
  })}
/>

<div class="pane-page">
  <PaneHeader
    title={m('admin.permissions.roles_page.title')}
    subtitle={m('admin.permissions.roles_page.subtitle')}
  />

  <PaneContent>
    <div class="flex flex-col gap-6">
      {#if !canManageRoles}
        <Hint tone="danger">{m('admin.permissions.need_manage_edit')}</Hint>
      {:else if rolesQuery.isPending}
        <LoadingFog class="h-48 w-full" />
      {:else if rolesQuery.error}
        <Hint tone="danger">{errorMessage(rolesQuery.error)}</Hint>
      {:else}
        <Hint>{m('admin.permissions.role_order.hint')}</Hint>

        <Panel title={m('admin.permissions.role_order.title')} noPadding>
          {#snippet actions()}
            <Button
              size="sm"
              variant="secondary"
              href={resolve('/chat/[serverId]/manage/server/roles/new', {
                serverId: serverSegment
              })}
            >
              <span aria-hidden="true" class="iconify icon-[uil--plus]"></span>
              {m('admin.permissions.create_role_action')}
            </Button>
          {/snippet}
          <div class="p-1">
            <div class="selectable-list panel-inset" aria-busy={saving}>
              {#if ownerRole}
                {@render fixedRow(ownerRole, m('admin.permissions.role_order.always_highest'))}
              {/if}
              <div
                class="flex flex-col gap-1"
                data-testid="role-order-dropzone"
                aria-label={m('admin.permissions.role_order.list_label')}
                use:dragHandleZone={{
                  items: movableItems,
                  flipDurationMs: 200,
                  dragDisabled: saving,
                  dropTargetStyle: {
                    outline: '2px dashed var(--color-action)',
                    'outline-offset': '-2px',
                    'border-radius': '0.5rem'
                  },
                  type: 'roles'
                }}
                onconsider={handleConsider}
                onfinalize={handleFinalize}
              >
                {#each movableItems as role (role.id)}
                  <div
                    animate:flip={{ duration: 200 }}
                    class="flex items-center gap-3 selectable-list-item py-2 ps-3 pe-4"
                    data-role={role.name}
                    aria-label={role.displayName}
                    data-sidebar-swipe-ignore
                  >
                    <span
                      use:dragHandle
                      data-sidebar-swipe-ignore
                      class="iconify icon-[uil--draggabledots] shrink-0 cursor-grab text-lg text-muted hover:text-text"
                      role="button"
                      aria-label={m('admin.permissions.role_order.drag_role', {
                        role: role.displayName
                      })}
                    ></span>
                    {@render roleRow(role, null)}
                  </div>
                {/each}
              </div>
              {#if everyoneRole}
                {@render fixedRow(everyoneRole, m('admin.permissions.role_order.always_lowest'))}
              {/if}
            </div>
          </div>
        </Panel>
      {/if}
    </div>
  </PaneContent>
</div>
