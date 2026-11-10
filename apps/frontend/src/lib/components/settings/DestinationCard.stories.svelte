<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import DestinationCard from './DestinationCard.svelte';
  const { Story } = defineMeta({ title: 'Settings/Destination card', component: DestinationCard });
</script>

<Story name="Help and settings" asChild>
  <div class="grid max-w-2xl grid-cols-1 gap-4 sm:grid-cols-2">
    <DestinationCard
      href="/help/conversations"
      label="Rooms and threads"
      description="Keep conversations organised."
      icon="iconify icon-[uil--comments]"
    />
    <DestinationCard
      href="/chat/preferences/appearance"
      label="Appearance"
      description="Choose the colours and theme for this device."
      icon="iconify icon-[uil--palette]"
    />
  </div>
</Story>
