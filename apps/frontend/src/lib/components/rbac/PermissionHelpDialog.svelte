<!--
@component
Explains one permission: its summary, detailed help, the levels where it can be
set, its privileged-mode requirement, and its inclusion relationships. `Dialog`
opens it as a bottom sheet in narrow touch windows. Related permissions are
buttons that switch the dialog to that permission.
-->
<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import { Dialog, Hint } from '$lib/ui';
  import { Button } from '$lib/ui/form';
  import {
    getIncludingPermissions,
    getPermissionCategory,
    getPermissionCategoryLabel,
    getPermissionDescription,
    PERMISSION_METADATA,
    type PermissionScope
  } from '$lib/permissions';

  let {
    visible = $bindable(false),
    permission = $bindable(null),
    permissions
  }: {
    visible?: boolean;
    /** Permission to explain. Keep it set while the dialog closes. */
    permission?: string | null;
    /** Permissions the server can configure here. Limits the related permissions shown. */
    permissions: readonly string[];
  } = $props();

  const SCOPE_LABELS: Record<PermissionScope, () => string> = {
    server: () => m('rbac.permissions.level_server'),
    group: () => m('rbac.permissions.level_group'),
    room: () => m('rbac.permissions.level_room'),
    dm: () => m('rbac.permissions.level_dm')
  };

  const metadata = $derived(permission ? PERMISSION_METADATA[permission] : undefined);
  const includes = $derived(
    (metadata?.includes ?? []).filter((included) => permissions.includes(included))
  );
  const includedBy = $derived(permission ? getIncludingPermissions(permissions, permission) : []);
</script>

{#snippet related(ids: string[])}
  <ul class="flex flex-wrap gap-2">
    {#each ids as id (id)}
      <li>
        <button type="button" class="cursor-pointer link" onclick={() => (permission = id)}
          >{id}</button
        >
      </li>
    {/each}
  </ul>
{/snippet}

<Dialog bind:visible title={permission ?? ''} size="sm">
  {#if permission}
    <div class="flex flex-col gap-4" data-testid="permission-help">
      {#if metadata}
        <p class="font-medium">{getPermissionDescription(permission)}</p>
        <p>{metadata.help()}</p>
      {/if}
      <dl class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-sm">
        <dt class="text-muted">{m('rbac.permissions.help.category')}</dt>
        <dd>{getPermissionCategoryLabel(getPermissionCategory(permission))}</dd>
        {#if metadata}
          <dt class="text-muted">{m('rbac.permissions.help.scopes')}</dt>
          <dd>{metadata.scopes.map((scope) => SCOPE_LABELS[scope]()).join(', ')}</dd>
        {/if}
        {#if includes.length > 0}
          <dt class="text-muted">{m('rbac.permissions.help.includes')}</dt>
          <dd>{@render related(includes)}</dd>
        {/if}
        {#if includedBy.length > 0}
          <dt class="text-muted">{m('rbac.permissions.help.included_by')}</dt>
          <dd>{@render related(includedBy)}</dd>
        {/if}
      </dl>
      {#if metadata?.privileged}
        <Hint icon="icon-[uil--shield]">
          <span class="font-medium">{m('rbac.permissions.help.privileged')}</span>
          {m('rbac.permissions.help.privileged_detail')}
        </Hint>
      {/if}
    </div>
  {/if}

  {#snippet primaryAction()}
    <Button defaultAction onclick={() => (visible = false)}>{m('common.got_it')}</Button>
  {/snippet}
</Dialog>
