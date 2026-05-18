<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import PaneHeader from './PaneHeader.svelte';
  import HeaderIconButton from './HeaderIconButton.svelte';

  const { Story } = defineMeta({
    title: 'UI/PaneHeader',
    component: PaneHeader,
    tags: ['autodocs']
  });
</script>

<Story name="Plain" asChild>
  <div class="w-[480px] rounded-md border border-border">
    <PaneHeader title="Members" subtitle="View and manage server members" />
  </div>
</Story>

<Story name="With back link (route-based)" asChild>
  <div class="w-[480px] rounded-md border border-border">
    <PaneHeader title="Permissions" subtitle="@moderator" backHref="#" backLabel="Back to role" />
  </div>
</Story>

<Story name="With back button (callback-based)" asChild>
  <div class="w-[480px] rounded-md border border-border">
    <PaneHeader title="Thread in #pico-8" onBack={() => {}} backLabel="Back to room">
      {#snippet actions()}
        <HeaderIconButton icon="uil--bell" label="Follow thread" tone="active" />
        <HeaderIconButton icon="uil--times" label="Close thread" />
      {/snippet}
    </PaneHeader>
  </div>
</Story>

<Story name="With actions" asChild>
  <div class="w-[480px] rounded-md border border-border">
    <PaneHeader title="#general" subtitle="Public · 142 members">
      {#snippet actions()}
        <HeaderIconButton icon="uil--bell" label="Notifications" />
        <HeaderIconButton icon="uil--sign-out-alt" label="Leave room" />
        <HeaderIconButton icon="uil--cog" label="Room settings" />
      {/snippet}
    </PaneHeader>
  </div>
</Story>

<Story name="Loading" asChild>
  <div class="w-[480px] rounded-md border border-border">
    <PaneHeader title="" loading skeletonButtons={2} />
  </div>
</Story>
