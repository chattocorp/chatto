<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import ActivityListRow from './ActivityListRow.svelte';

  const { Story } = defineMeta({
    title: 'UI/Activity list row',
    component: ActivityListRow,
    tags: ['autodocs']
  });
</script>

<Story name="Compact" asChild>
  <div class="max-w-xl">
    {#each ['Mention in general', 'Reply in design', 'Reaction in development'] as summary (summary)}
      <ActivityListRow compact onclick={() => undefined}>
        {#snippet leading()}
          <span class="size-8 shrink-0 rounded-full bg-surface-emphasized"></span>
        {/snippet}
        <span class="min-w-0 flex-1 truncate">{summary}</span>
        {#snippet actions()}
          <button type="button" class="icon-action" aria-label="Dismiss">
            <span class="iconify icon-[uil--trash-alt]" aria-hidden="true"></span>
          </button>
        {/snippet}
      </ActivityListRow>
    {/each}
  </div>
</Story>

<Story name="Overlay actions" asChild>
  <ActivityListRow compact overlayActions onclick={() => undefined}>
    <span class="min-w-0 flex-1 truncate">Mention in general</span>
    <span
      class="text-sm text-muted [@media(hover:hover)]:group-focus-within:opacity-0 [@media(hover:hover)]:group-hover:opacity-0"
      >2m ago</span
    >
    {#snippet actions()}
      <button type="button" class="icon-action" aria-label="Dismiss">
        <span class="iconify icon-[uil--trash-alt]" aria-hidden="true"></span>
      </button>
    {/snippet}
  </ActivityListRow>
</Story>

<Story name="States" asChild>
  <div class="selectable-list max-w-xl">
    <ActivityListRow onclick={() => undefined}>
      {#snippet leading()}
        <span class="flex h-10 w-10 items-center justify-center rounded-full bg-surface-emphasized">
          <span class="iconify icon-[uil--comment-alt-lines]" aria-hidden="true"></span>
        </span>
      {/snippet}
      <span class="min-w-0 flex-1">
        <span class="block truncate font-medium">New activity</span>
        <span class="block truncate text-sm text-muted">General · just now</span>
      </span>
      {#snippet actions()}
        <button type="button" class="icon-action" aria-label="Dismiss">
          <span class="iconify icon-[uil--trash-alt]" aria-hidden="true"></span>
        </button>
      {/snippet}
    </ActivityListRow>

    <ActivityListRow pending disabled onclick={() => undefined}>
      <span class="min-w-0 flex-1">
        <span class="block truncate font-medium">Pending activity</span>
        <span class="block truncate text-sm text-muted">Waiting for the server</span>
      </span>
    </ActivityListRow>

    <ActivityListRow important onclick={() => undefined}>
      <span class="min-w-0 flex-1">
        <span class="block truncate font-medium">Important activity</span>
        <span class="block truncate text-sm text-muted">Needs attention</span>
      </span>
    </ActivityListRow>
  </div>
</Story>
