<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import RoomName from './RoomName.svelte';

  const { Story } = defineMeta({
    title: 'Chat/Room name',
    component: RoomName,
    tags: ['autodocs']
  });
</script>

<Story name="Channel" args={{ name: 'general' }} />
<Story name="Universal room" args={{ name: 'announcements', isUniversal: true }} />
<Story name="Long name" asChild>
  <div class="w-48 font-semibold">
    <RoomName name="Development questions and community support" />
  </div>
</Story>
<Story name="Right-to-left name" args={{ name: 'النقاش العام' }} />
