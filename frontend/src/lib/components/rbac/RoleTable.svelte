<script lang="ts">
  import { dndzone, type DndEvent } from 'svelte-dnd-action';
  import { flip } from 'svelte/animate';
  import { Button } from '$lib/ui/form';
  import { Pill } from '$lib/ui';
  import type { Role } from './types';

  let {
    roles,
    canManage = false,
    adminRoleName = 'admin',
    onEdit,
    onReorder
  }: {
    roles: Role[];
    canManage?: boolean;
    adminRoleName?: string;
    onEdit?: (role: Role) => void;
    onReorder?: (roleNames: string[]) => void;
  } = $props();

  // Items for drag-and-drop need an id property
  type DraggableRole = Role & { id: string };

  // Sort roles by position and add id for dnd
  // eslint-disable-next-line svelte/prefer-writable-derived -- handlers mutate sortedRoles during drag
  let sortedRoles = $state<DraggableRole[]>([]);

  $effect(() => {
    sortedRoles = [...roles]
      .sort((a, b) => a.position - b.position)
      .map((r) => ({ ...r, id: r.name }));
  });

  // Check if a role can be dragged (only custom roles)
  function isDraggable(role: Role): boolean {
    return canManage && !!onReorder && !role?.isSystem;
  }

  // Handle drag events
  function handleConsider(e: CustomEvent<DndEvent<DraggableRole>>) {
    sortedRoles = e.detail.items;
  }

  function handleFinalize(e: CustomEvent<DndEvent<DraggableRole>>) {
    sortedRoles = e.detail.items;

    // Extract just the custom role names in the new order
    const customRoleNames = sortedRoles.filter((r) => !r.isSystem).map((r) => r.name);

    onReorder?.(customRoleNames);
  }

  // Whether drag-and-drop is enabled
  const dndEnabled = $derived(canManage && !!onReorder);

  // Get number of columns for the table
  const columnCount = $derived((canManage && onEdit ? 6 : 5) + (dndEnabled ? 1 : 0));
</script>

<table class="w-full border-collapse">
  <thead>
    <tr class="border-b border-border bg-surface-200/50">
      {#if dndEnabled}
        <th class="w-8 px-2 py-3"></th>
      {/if}
      <th class="px-4 py-3 text-left text-sm font-medium">Name</th>
      <th class="px-4 py-3 text-left text-sm font-medium">Display Name</th>
      <th class="px-4 py-3 text-left text-sm font-medium">Description</th>
      <th class="px-4 py-3 text-center text-sm font-medium">Permissions</th>
      <th class="px-4 py-3 text-center text-sm font-medium">Type</th>
      {#if canManage && onEdit}
        <th class="px-4 py-3 text-center text-sm font-medium">Actions</th>
      {/if}
    </tr>
  </thead>
  <tbody
    use:dndzone={{
      items: sortedRoles,
      flipDurationMs: 200,
      dropTargetStyle: {},
      dragDisabled: !dndEnabled
    }}
    onconsider={handleConsider}
    onfinalize={handleFinalize}
  >
    {#each sortedRoles as role (role.id)}
      <tr
        animate:flip={{ duration: 200 }}
        class={[
          'border-b border-border bg-surface last:border-b-0',
          canManage && onEdit ? 'cursor-pointer hover:bg-surface-200' : '',
          role.isSystem ? 'opacity-75' : ''
        ]}
        onclick={() => canManage && onEdit?.(role)}
      >
        {#if dndEnabled}
          <td class="w-8 px-2 py-3 text-center">
            {#if isDraggable(role)}
              <span
                role="button"
                tabindex="0"
                class="hover:text-foreground cursor-grab text-muted"
                title="Drag to reorder"
                aria-label="Drag to reorder"
                onclick={(e) => e.stopPropagation()}
                onkeydown={(e) => e.key === 'Enter' && e.stopPropagation()}
              >
                <span class="iconify text-lg uil--draggabledots"></span>
              </span>
            {:else}
              <span class="text-muted/30" title="System roles cannot be reordered">
                <span class="iconify text-lg uil--lock"></span>
              </span>
            {/if}
          </td>
        {/if}
        <td class="px-4 py-3">
          <code class="text-sm">{role.name}</code>
        </td>
        <td class="px-4 py-3">{role.displayName}</td>
        <td class="px-4 py-3 text-sm text-muted">{role.description}</td>
        <td class="px-4 py-3 text-center">
          {#if role.name === adminRoleName}
            <span class="text-muted">All</span>
          {:else}
            {role.permissions?.length ?? 0}
          {/if}
        </td>
        <td class="px-4 py-3 text-center">
          <Pill tone={role.isSystem ? 'muted' : 'primary'}>
            {role.isSystem ? 'System' : 'Custom'}
          </Pill>
        </td>
        {#if canManage && onEdit}
          <td class="px-4 py-3 text-center">
            <Button
              variant="secondary"
              size="sm"
              onclick={(e) => {
                e.stopPropagation();
                onEdit(role);
              }}
            >
              Edit
            </Button>
          </td>
        {/if}
      </tr>
    {:else}
      <tr>
        <td colspan={columnCount} class="px-4 py-8 text-center text-muted"> No roles found </td>
      </tr>
    {/each}
  </tbody>
</table>
