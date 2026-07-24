<script module lang="ts">
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { type UserAvatarUserView } from '$lib/render/types';
  import UserAvatar from './UserAvatar.svelte';

  const { Story } = defineMeta({
    title: 'Components/UserAvatar',
    component: UserAvatar,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: {
          component: 'Circular user avatar rendering with optional presence dots.'
        }
      }
    }
  });

  function user(
    id: string,
    displayName: string,
    presenceStatus: PresenceStatus
  ): UserAvatarUserView {
    return {
      id,
      login: id,
      displayName,
      deleted: false,
      avatarUrl: null,
      presenceStatus,
      customStatus: null
    };
  }

  const onlineUser = user('online', 'Online User', PresenceStatus.ONLINE);
  const awayUser = user('away', 'Away User', PresenceStatus.AWAY);
  const dndUser = user('dnd', 'DND User', PresenceStatus.DO_NOT_DISTURB);
  const offlineUser = user('offline', 'Offline User', PresenceStatus.OFFLINE);
</script>

<script lang="ts">
  import { createPresenceCache } from '$lib/state/presenceCache.svelte';
  import { createUserProfileCache } from '$lib/state/userProfiles.svelte';

  createUserProfileCache();
  createPresenceCache();
</script>

<Story name="Presence dots" asChild>
  <div class="flex items-center gap-5 rounded-md bg-surface p-4">
    <UserAvatar user={onlineUser} size="md" showPresence />
    <UserAvatar user={awayUser} size="md" showPresence />
    <UserAvatar user={dndUser} size="md" showPresence />
    <UserAvatar user={offlineUser} size="md" showPresence />
  </div>
</Story>

<Story name="Plain avatars" asChild>
  <div class="flex items-center gap-4 rounded-md bg-surface p-4">
    <UserAvatar user={onlineUser} size="xs" />
    <UserAvatar user={awayUser} size="sm" />
    <UserAvatar user={dndUser} size="md" />
  </div>
</Story>
