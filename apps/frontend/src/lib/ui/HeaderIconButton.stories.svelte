<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import HeaderIconButton from './HeaderIconButton.svelte';

  const componentDescription = `
    Use HeaderIconButton for compact icon-only actions in pane headers and tight toolbars. Provide
    a clear label for accessibility and prefer the active tone for toggled-on state, not hover-only
    emphasis.
  `.trim();

  const { Story } = defineMeta({
    title: 'UI/HeaderIconButton',
    component: HeaderIconButton,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: { component: componentDescription }
      }
    }
  });
</script>

<Story name="Tones" asChild>
  <div class="flex items-center gap-3">
    <HeaderIconButton icon="uil--bell" label="Default" />
    <HeaderIconButton icon="uil--bell" label="Active (toggled on)" tone="active" />
    <HeaderIconButton icon="uil--trash" label="Danger" tone="danger" />
    <HeaderIconButton icon="uil--cog" label="Disabled" disabled />
  </div>
</Story>

<Story name="As link" asChild>
  <div class="flex items-center gap-3">
    <HeaderIconButton icon="uil--cog" label="Settings" href="#" />
    <HeaderIconButton icon="uil--question-circle" label="Help" href="#" />
  </div>
</Story>

<Story name="Common pane-header actions" asChild>
  <div class="flex items-center gap-3 rounded-md border border-border bg-surface p-3">
    <HeaderIconButton icon="uil--bell" label="Follow thread" tone="active" />
    <HeaderIconButton icon="uil--sign-out-alt" label="Leave room" />
    <HeaderIconButton icon="uil--cog" label="Settings" />
    <HeaderIconButton icon="uil--times" label="Close" />
  </div>
</Story>

<Story name="Call pane actions" asChild>
  <div class="flex items-center gap-3 rounded-md border border-border bg-surface p-3">
    <HeaderIconButton icon="mdi--arrow-expand-left" label="Maximize call" />
    <HeaderIconButton icon="mdi--arrow-collapse-right" label="Minimize call" tone="active" />
    <HeaderIconButton icon="mdi--monitor-share" label="Fullscreen call" />
    <HeaderIconButton icon="mdi--fullscreen-exit" label="Exit fullscreen call" tone="active" />
    <HeaderIconButton icon="uil--times" label="Close" iconSize="lg" />
  </div>
</Story>
