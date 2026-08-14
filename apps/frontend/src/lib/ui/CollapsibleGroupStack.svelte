<!--
@component

A stack of sidebar `CollapsibleGroup` sections with the standard full-width
divider between adjacent groups. Place it inside a container with `p-2` so the
divider bleeds to the container edge while group content keeps its normal inset.
-->
<script module lang="ts">
  import type { Snippet } from 'svelte';
  import type { Attachment } from 'svelte/attachments';

  export type CollapsibleGroupStackEntry<T extends { id: string }> = {
    id: string;
    label: string;
    items: T[];
    persistKey: string;
    defaultCollapsed?: boolean;
    keepVisibleWhenCollapsed?: (item: T) => boolean;
    actions?: Snippet;
    contextMenuTrigger?: Attachment<HTMLElement>;
    testid?: string;
  };
</script>

<script lang="ts" generics="T extends { id: string }">
  import CollapsibleGroup from './CollapsibleGroup.svelte';

  let {
    groups,
    item,
    class: className
  }: {
    groups: CollapsibleGroupStackEntry<T>[];
    item: Snippet<[T]>;
    class?: string;
  } = $props();
</script>

<div class={['flex flex-col', className]}>
  {#each groups as group, i (group.id)}
    {#if i > 0}
      <hr class="-mx-2 my-2 border-border" data-testid="collapsible-group-separator" />
    {/if}
    <CollapsibleGroup
      label={group.label}
      items={group.items}
      {item}
      actions={group.actions}
      contextMenuTrigger={group.contextMenuTrigger}
      persistKey={group.persistKey}
      defaultCollapsed={group.defaultCollapsed}
      keepVisibleWhenCollapsed={group.keepVisibleWhenCollapsed}
      testid={group.testid}
    />
  {/each}
</div>
