<script module lang="ts">
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import type { UserAvatarUserView } from '$lib/render/users';
  import imageAvatar from '$lib/assets/chatto-icon-maskable.png';
  import botIcon from '$lib/assets/bot.svg';
  import UserAvatar from './UserAvatar.svelte';

  const { Story } = defineMeta({
    title: 'Components/UserAvatar',
    component: UserAvatar,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: {
          component: 'Circular user avatars with optional presence dots and coloured bot icons.'
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
    <UserAvatar user={onlineUser} serverId="storybook" size="md" showPresence />
    <UserAvatar user={awayUser} serverId="storybook" size="md" showPresence />
    <UserAvatar user={dndUser} serverId="storybook" size="md" showPresence />
    <UserAvatar user={offlineUser} serverId="storybook" size="md" showPresence />
  </div>
</Story>

<Story name="Plain avatars" asChild>
  <div class="flex items-center gap-4 rounded-md bg-surface p-4">
    <UserAvatar user={onlineUser} size="xs" />
    <UserAvatar user={awayUser} size="sm" />
    <UserAvatar user={dndUser} size="md" />
  </div>
</Story>

<Story name="Bot accounts" asChild>
  <div class="flex flex-wrap items-center gap-6 rounded-md bg-surface p-6">
    {#each ['xs', 'sm', 'md', 'message', 'lg', 'xl'] as const as size (size)}
      <div class="flex flex-col items-center gap-3">
        <UserAvatar user={{ ...onlineUser, displayName: 'Assistant', isBot: true }} {size} />
        <span class="text-xs text-muted">{size}</span>
      </div>
    {/each}
  </div>
</Story>

<Story name="Bots with status" asChild>
  <div class="flex items-center gap-6 rounded-md bg-surface p-6">
    <UserAvatar
      user={{ ...onlineUser, displayName: 'Assistant', isBot: true }}
      size="sm"
      showPresence
    />
    <UserAvatar
      user={{
        ...onlineUser,
        displayName: 'Assistant',
        isBot: true,
        customStatus: { emoji: '🔧', text: 'Building', expiresAt: null }
      }}
      size="message"
      showPresence
      showStatus
    />
    <UserAvatar user={onlineUser} size="message" showPresence />
  </div>
</Story>

<Story name="Robot artwork" asChild>
  <div class="flex max-w-2xl items-center justify-center gap-16 rounded-xl bg-surface px-10 py-12">
    <img src={botIcon} alt="Chatto robot" class="size-40 outline-none" />
    <div class="flex flex-col items-center gap-6">
      <div class="flex items-center gap-6">
        <UserAvatar user={{ ...onlineUser, displayName: 'Assistant', isBot: true }} size="sm" />
        <UserAvatar
          user={{ ...onlineUser, displayName: 'Assistant', isBot: true }}
          size="message"
          showPresence
        />
        <UserAvatar user={{ ...onlineUser, displayName: 'Assistant', isBot: true }} size="xl" />
      </div>
      <div class="flex items-center gap-6">
        {#each ['xs', 'sm', 'message'] as const as size (size)}
          <UserAvatar
            user={{ ...onlineUser, displayName: 'Assistant', isBot: true, avatarUrl: imageAvatar }}
            {size}
          />
        {/each}
      </div>
      <span class="text-sm text-muted">Actual avatar sizes</span>
    </div>
  </div>
</Story>
