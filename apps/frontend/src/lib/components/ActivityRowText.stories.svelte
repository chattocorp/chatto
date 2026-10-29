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
  <div class="selectable-list max-w-5xl">
    <ActivityListRow rowAttributes={{ class: '@container/activity' }} onclick={() => undefined}>
      <ActivityRowText time="2m ago" emphasized>
        {#snippet title()}Unread activity is bold{/snippet}
        {#snippet location()}<bdi dir="auto">#general</bdi>{/snippet}
      </ActivityRowText>
    </ActivityListRow>
    <ActivityListRow rowAttributes={{ class: '@container/activity' }} onclick={() => undefined}>
      <ActivityRowText time="4h ago">
        {#snippet title()}Read activity uses muted text{/snippet}
        {#snippet location()}<bdi dir="auto">#design</bdi>{/snippet}
      </ActivityRowText>
    </ActivityListRow>
    <ActivityListRow onclick={() => undefined}>
      <ActivityRowText time="Yesterday">
        {#snippet title()}A row without a location{/snippet}
      </ActivityRowText>
    </ActivityListRow>
  </div>
</Story>
