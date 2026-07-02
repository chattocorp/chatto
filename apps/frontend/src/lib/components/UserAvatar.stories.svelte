<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { PresenceStatus, type UserAvatarUserView } from '$lib/render/types';
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

  const onlineUser = user('online', 'Online User', PresenceStatus.Online);
  const awayUser = user('away', 'Away User', PresenceStatus.Away);
  const dndUser = user('dnd', 'DND User', PresenceStatus.DoNotDisturb);
  const offlineUser = user('offline', 'Offline User', PresenceStatus.Offline);
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
