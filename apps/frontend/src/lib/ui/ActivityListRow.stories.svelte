<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import ActivityListRow from './ActivityListRow.svelte';

  const { Story } = defineMeta({
    title: 'UI/Activity list row',
    component: ActivityListRow,
    tags: ['autodocs']
  });
</script>

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

    <ActivityListRow dimmed onclick={() => undefined}>
      <span class="min-w-0 flex-1">
        <span class="block truncate font-medium">Read activity</span>
        <span class="block truncate text-sm text-muted">Yesterday</span>
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
