<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { ActivityListRow } from '$lib/ui';
  import ActivityRowText from './ActivityRowText.svelte';

  const { Story } = defineMeta({
    title: 'Components/Activity row text',
    component: ActivityRowText,
    tags: ['autodocs']
  });
</script>

<Story name="Variants" asChild>
  <div class="selectable-list max-w-xl">
    <ActivityListRow onclick={() => undefined}>
      <ActivityRowText time="2m ago" emphasized>
        {#snippet title()}Unread activity is bold{/snippet}
        {#snippet subtitle()}#general{/snippet}
      </ActivityRowText>
    </ActivityListRow>
    <ActivityListRow onclick={() => undefined}>
      <ActivityRowText time="4h ago" trailing="3 replies">
        {#snippet title()}Read activity keeps the normal text color{/snippet}
        {#snippet subtitle()}#design · Context for the activity{/snippet}
      </ActivityRowText>
    </ActivityListRow>
    <ActivityListRow onclick={() => undefined}>
      <ActivityRowText time="Yesterday">
        {#snippet title()}A row without a subtitle{/snippet}
      </ActivityRowText>
    </ActivityListRow>
  </div>
</Story>
