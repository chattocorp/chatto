<script lang="ts">
  import type { ComponentProps } from 'svelte';
  import type { UserStore } from '@chatto/client/server/users';
  import { provideUserProfiles } from '$lib/state/userProfiles.svelte';
  import MessageView from './MessageView.svelte';

  let { users, ...props }: ComponentProps<typeof MessageView> & { users?: UserStore } = $props();

  provideUserProfiles(() => users);
</script>

<MessageView {...props} />
