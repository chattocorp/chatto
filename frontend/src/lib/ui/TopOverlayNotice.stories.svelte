<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import TopOverlayNotice from './TopOverlayNotice.svelte';

  const componentDescription = `
    Use TopOverlayNotice for temporary, high-priority prompts that sit above the current pane
    content, such as permission prompts. It should remain concise and offer no more than one primary
    and one secondary action.
  `.trim();

  const { Story } = defineMeta({
    title: 'UI/TopOverlayNotice',
    component: TopOverlayNotice,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: { component: componentDescription }
      }
    }
  });
</script>

<Story name="Push prompt" asChild>
  <div class="relative h-80 overflow-hidden rounded-lg border border-border bg-background">
    <TopOverlayNotice
      title="Enable push notifications"
      message="Get notified about DMs, mentions, and replies."
      icon="uil--bell"
      primaryAction={{
        label: 'Enable',
        icon: 'uil--bell',
        onclick: () => {}
      }}
      secondaryAction={{
        label: 'No thanks',
        onclick: () => {}
      }}
    />
  </div>
</Story>
