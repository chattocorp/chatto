<script module lang="ts">
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import type { UserAvatarUserView } from '$lib/render/users';
  import UserAvatarStack from './UserAvatarStack.svelte';

  const users: UserAvatarUserView[] = [
    {
      id: 'user-1',
      login: 'alex',
      displayName: 'Alex Rivera',
      deleted: false,
      presenceStatus: PresenceStatus.ONLINE
    },
    {
      id: 'user-2',
      login: 'sam',
      displayName: 'Sam Chen',
      deleted: false,
      presenceStatus: PresenceStatus.AWAY
    }
  ];

  const { Story } = defineMeta({
    title: 'Components/User avatar stack',
    component: UserAvatarStack,
    tags: ['autodocs']
  });
</script>

<Story name="Single" args={{ users: users.slice(0, 1), useLiveProfile: false }} />
<Story name="Group" args={{ users, useLiveProfile: false }} />
