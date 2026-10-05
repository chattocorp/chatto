<!--
@component

Overview section of a bot: its ID, owner, public profile, and avatar, with the
owner reassignment and deletion actions.
-->
<script lang="ts">
  import { errorMessage, toastError } from '$lib/utils/errorMessage';
  import AccountName from '$lib/components/users/AccountName.svelte';
  import { accountNameToken } from '@chatto/client/timeline/accountName';
  import AccountNameTokens from '$lib/components/users/AccountNameTokens.svelte';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
  import { createBotAPI } from '@chatto/client/api/bots';
  import {
    createUserAPI,
    type UpdateUserProfileInput,
    type UserSummary
  } from '@chatto/client/api/users';
  import { CopyId, Panel, ConfirmDialog, FormDialog, Hint, LoadingFog } from '$lib/ui';
  import BotProfileSection from '$lib/components/bots/BotProfileSection.svelte';
  import AvatarEditor from '$lib/components/users/AvatarEditor.svelte';
  import UserCombobox from '$lib/components/users/UserCombobox.svelte';
  import UserIdentity from '$lib/components/users/UserIdentity.svelte';
  import { m } from '$lib/i18n/messages';
  import { serverIdToSegment } from '$lib/navigation';
  import { createQuery, queryClient } from '$lib/query/client';
  import { adminQueryKeys } from '$lib/query/admin';
  import { settingsQueryKeys } from '$lib/query/settings';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { Button } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';
  import { startsLoginCooldown } from '$lib/validation';
  import { useBotDetail } from './botDetailContext';

  const serverScope = useServerScope();
  const detail = useBotDetail();
  const bot = $derived(detail.bot);
  const canManageAccounts = $derived(serverScope.store.permissions.canAdminManageAccounts);
  const canReassignOwner = $derived(serverScope.store.permissions.canManageBots);

  const ownerQuery = createQuery(() => {
    const serverId = serverScope.serverId;
    const connection = serverScope.connection;
    const ownerUserId = bot?.ownerUserId ?? '';
    return {
      queryKey: [
        ...settingsQueryKeys.bot(serverId, connection, detail.botId),
        'owner',
        ownerUserId
      ],
      queryFn: () => connection.getAPI(createUserAPI).batchGetUsers([ownerUserId]),
      enabled: !!ownerUserId
    };
  });
  const owner = $derived(ownerQuery.data?.[0] ?? null);

  let deleteVisible = $state(false);
  let deleteLoading = $state(false);
  let reassignVisible = $state(false);
  let reassignOwnerUserId = $state('');
  let reassignOwnerText = $state('');
  let reassignLoading = $state(false);
  let reassignError = $state<string | null>(null);

  function botAPI() {
    return serverScope.connection.getAPI(createBotAPI);
  }

  function userAPI() {
    return serverScope.connection.getAPI(createUserAPI);
  }

  async function updateProfile(input: UpdateUserProfileInput): Promise<UserSummary | null> {
    if (!bot) return null;
    const mutationTarget = detail.botId;
    // Account managers bypass the cooldown; other renames start a new one.
    const startedCooldown =
      input.login !== undefined &&
      !canManageAccounts &&
      startsLoginCooldown(bot.login, input.login);
    const updated = await userAPI().updateUserProfile(bot.id, input);
    if (!detail.isCurrentTarget(mutationTarget) || !bot) return null;
    detail.cacheBot({
      ...bot,
      login: updated.login,
      displayName: updated.displayName,
      bio: updated.bio ?? null,
      lastLoginChange: startedCooldown ? new Date() : bot.lastLoginChange
    });
    return updated;
  }

  async function uploadAvatar(file: File): Promise<boolean> {
    if (!bot) return false;
    const mutationTarget = detail.botId;
    const updated = await userAPI().uploadAvatar(bot.id, file);
    if (!detail.isCurrentTarget(mutationTarget) || !bot) return false;
    detail.cacheBot({ ...bot, avatarUrl: updated.avatarUrl });
    return true;
  }

  async function deleteAvatar(): Promise<boolean> {
    if (!bot) return false;
    const mutationTarget = detail.botId;
    const updated = await userAPI().deleteAvatar(bot.id);
    if (!detail.isCurrentTarget(mutationTarget) || !bot) return false;
    detail.cacheBot({ ...bot, avatarUrl: updated.avatarUrl });
    return true;
  }

  function openReassignOwner() {
    reassignOwnerUserId = '';
    reassignOwnerText = '';
    reassignError = null;
    reassignVisible = true;
  }

  async function reassignOwner() {
    if (!bot || !canReassignOwner || reassignOwnerUserId === bot.ownerUserId) return;
    const mutationTarget = detail.botId;
    reassignLoading = true;
    reassignError = null;
    try {
      const reassigned = await botAPI().reassignBotOwner(bot.id, reassignOwnerUserId);
      if (!detail.isCurrentTarget(mutationTarget)) return;
      detail.cacheBot(reassigned);
      void queryClient.invalidateQueries({
        queryKey: adminQueryKeys.userPermissions(
          serverScope.serverId,
          serverScope.connection,
          bot.id
        ),
        exact: true
      });
      reassignVisible = false;
      toast.success(m('settings.bots.owner_reassigned'));
    } catch (error) {
      if (!detail.isCurrentTarget(mutationTarget)) return;
      reassignError = errorMessage(error, m('settings.bots.owner_reassign_failed'));
    } finally {
      if (detail.isCurrentTarget(mutationTarget)) reassignLoading = false;
    }
  }

  async function deleteBot() {
    if (!bot) return;
    const mutationTarget = detail.botId;
    deleteLoading = true;
    try {
      await botAPI().deleteBot(bot.id);
      if (!detail.isCurrentTarget(mutationTarget)) return;
      queryClient.removeQueries({
        queryKey: settingsQueryKeys.bot(serverScope.serverId, serverScope.connection, bot.id),
        exact: true
      });
      detail.refreshBot();
      toast.success(m('settings.bots.deleted'));
      await goto(
        resolve('/chat/[serverId]/manage/server/bots', {
          serverId: serverIdToSegment(serverScope.serverId)
        })
      );
    } catch (error) {
      if (detail.isCurrentTarget(mutationTarget)) {
        toastError(error, m('settings.bots.delete_failed'));
      }
    } finally {
      if (detail.isCurrentTarget(mutationTarget)) deleteLoading = false;
    }
  }
</script>

{#snippet botName()}<AccountName
    name={bot?.displayName ?? m('settings.bots.title')}
    identity={bot ? { isBot: true } : undefined}
  />{/snippet}

{#if bot}
  <Panel title={bot.displayName} titleContent={botName} subtitle={`@${bot.login}`}>
    {#snippet actions()}
      {#if canReassignOwner}
        <Button size="sm" variant="secondary" onclick={openReassignOwner}>
          <span class="iconify icon-[uil--exchange]" aria-hidden="true"></span>
          {m('settings.bots.reassign_owner')}
        </Button>
      {/if}
      {#if detail.canOperateBot}
        <Button size="sm" variant="danger-secondary" onclick={() => (deleteVisible = true)}>
          <span class="iconify icon-[uil--trash]" aria-hidden="true"></span>
          {m('common.delete')}
        </Button>
      {/if}
    {/snippet}
    <dl class="grid gap-4 text-sm sm:grid-cols-2 lg:grid-cols-4">
      <div>
        <dt class="text-muted">{m('admin.members.user_id')}</dt>
        <dd class="mt-1"><CopyId value={bot.id} /></dd>
      </div>
      <div>
        <dt class="text-muted">{m('settings.bots.owner')}</dt>
        <dd class="mt-1">
          {#if owner}
            <UserIdentity
              user={{ ...owner, presenceStatus: PresenceStatus.OFFLINE }}
              viewerSettings={serverScope.store.currentUser.user?.settings}
            />
          {:else if ownerQuery.isPending}
            <LoadingFog class="h-5 w-28" />
          {:else}
            <span class="text-muted">{m('common.unknown')}</span>
          {/if}
        </dd>
      </div>
    </dl>
  </Panel>

  {#if detail.canEditIdentity}
    {#key detail.botId}
      <BotProfileSection {bot} canBypassLoginCooldown={canManageAccounts} onsave={updateProfile} />
      <AvatarEditor
        user={{ ...bot, isBot: true }}
        onupload={uploadAvatar}
        ondelete={deleteAvatar}
      />
    {/key}
  {/if}
{:else if detail.isPending}
  <LoadingFog class="h-40 w-full" />
{/if}

<FormDialog
  bind:visible={reassignVisible}
  title={m('settings.bots.reassign_owner')}
  submitLabel={m('settings.bots.reassign_owner')}
  loading={reassignLoading}
  disabled={!reassignOwnerUserId || reassignOwnerUserId === bot?.ownerUserId}
  error={reassignError}
  onsubmit={reassignOwner}
  onclose={() => (reassignVisible = false)}
>
  <Hint tone="warning">{m('settings.bots.reassign_owner_warning')}</Hint>
  <UserCombobox
    id="reassign-bot-owner"
    label={m('settings.bots.owner')}
    placeholder={m('admin.members.search_placeholder')}
    humanOnly
    allowFreeform={false}
    bind:value={reassignOwnerUserId}
    bind:text={reassignOwnerText}
  />
</FormDialog>

<ConfirmDialog
  bind:visible={deleteVisible}
  title={m('settings.bots.delete_title')}
  actionLabel={m('common.delete')}
  loading={deleteLoading}
  onconfirm={deleteBot}
  onclose={() => (deleteVisible = false)}
>
  <AccountNameTokens
    text={m('settings.bots.delete_warning', { name: bot ? accountNameToken(0) : '' })}
    accounts={bot ? [{ name: bot.displayName, identity: { isBot: true } }] : []}
  />
</ConfirmDialog>
