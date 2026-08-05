<script lang="ts">
  import type { AdminMember, AdminRoleDetails } from '$lib/api-client/adminUsers';
  import { CopyId, Panel } from '$lib/components/admin';
  import { m } from '$lib/i18n/messages';
  import { getLocale } from '$lib/i18n/runtime';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { getLiveLogin } from '$lib/state/userProfiles.svelte';
  import { Pill } from '$lib/ui';
  import { formatDate, formatDateTime, timeFormatSettingsFor } from '$lib/utils/formatTime';
  import { getAvatarInitials } from '$lib/utils/initials';
  import { formatCooldownRemaining, getLoginChangeCooldownRemaining } from '$lib/validation';

  type Props = {
    member: AdminMember;
    roles: AdminRoleDetails[];
    canViewMemberEmails: boolean;
  };

  let { member, roles, canViewMemberEmails }: Props = $props();

  const serverScope = useServerScope();
  const userSettings = $derived(
    timeFormatSettingsFor(serverScope.store.currentUser.user?.settings)
  );
  const activeLocale = $derived(getLocale());
  const lastLoginChange = $derived(
    member.lastLoginChange ? new Date(member.lastLoginChange) : null
  );
  const cooldownRemaining = $derived(getLoginChangeCooldownRemaining(lastLoginChange));
  const cooldownActive = $derived(cooldownRemaining > 0);
  const sortedServerRoles = $derived(
    member.roles
      .filter((roleName) => roleName !== 'everyone')
      .sort((a, b) => rolePosition(a) - rolePosition(b))
  );
  const serverRoleCount = $derived(sortedServerRoles.length);
  const cooldownSummary = $derived.by(() => {
    if (cooldownActive) {
      return m('admin.member_detail.self_rename_cooldown', {
        remaining: formatCooldownRemaining(cooldownRemaining)
      });
    }
    if (lastLoginChange) {
      return m('admin.member_detail.last_self_rename', {
        time: formatDateTime(lastLoginChange, userSettings, activeLocale)
      });
    }
    return m('admin.member_detail.no_self_rename');
  });

  function roleDisplayName(roleName: string): string {
    return roles.find((role) => role.name === roleName)?.displayName ?? roleName;
  }

  function rolePosition(roleName: string): number {
    return roles.find((role) => role.name === roleName)?.position ?? Number.MAX_SAFE_INTEGER;
  }

  function formatOptionalDate(date: string | null | undefined): string {
    return date ? formatDate(date, userSettings, activeLocale) : m('admin.common.unknown');
  }

  function emailSummary(): string {
    if (!canViewMemberEmails) return m('admin.member_detail.email_unavailable');
    if (member.verifiedEmails.length > 0) return member.verifiedEmails.join(', ');
    if (member.hasVerifiedEmail) return m('admin.member_detail.verified_email_on_file');
    return m('admin.member_detail.no_verified_email');
  }
</script>

<Panel title={m('admin.members.user_details')} icon="iconify icon-[uil--user]">
  <div class="flex flex-col gap-6">
    <div class="flex flex-col gap-4 sm:flex-row sm:items-start">
      {#if member.avatarUrl}
        <img
          src={member.avatarUrl}
          alt={member.displayName}
          class="h-20 w-20 rounded-full border border-border object-cover"
        />
      {:else}
        <div
          class="flex h-20 w-20 shrink-0 items-center justify-center rounded-full bg-surface-strong text-3xl text-muted"
        >
          {getAvatarInitials(member.displayName, member.login)}
        </div>
      {/if}

      <div class="min-w-0 flex-1">
        <div class="flex flex-col gap-1">
          <h3 class="truncate text-2xl font-semibold">{member.displayName}</h3>
          <div class="truncate text-muted">@{getLiveLogin(member.id, member.login)}</div>
        </div>

        <div class="mt-4 flex flex-wrap gap-2">
          {#if member.deleted}
            <Pill tone="danger">{m('admin.members.deleted_account')}</Pill>
          {:else}
            <Pill tone="success">{m('admin.members.member')}</Pill>
          {/if}
          {#if canViewMemberEmails}
            <Pill tone={member.hasVerifiedEmail ? 'success' : 'muted'}>
              {member.hasVerifiedEmail
                ? m('admin.members.email_verified')
                : m('admin.members.email_not_verified')}
            </Pill>
          {:else}
            <Pill tone="muted">{m('admin.members.email_hidden')}</Pill>
          {/if}
          <Pill tone={serverRoleCount > 0 ? 'neutral' : 'muted'}>
            {serverRoleCount === 1
              ? m('admin.members.server_role_one')
              : m('admin.members.server_role_many', { count: serverRoleCount })}
          </Pill>
          <Pill tone={member.viewerCanDeleteAccount ? 'danger' : 'muted'}>
            {member.viewerCanDeleteAccount
              ? m('admin.members.deletion_allowed')
              : m('admin.members.deletion_protected')}
          </Pill>
          <Pill tone={cooldownActive ? 'action' : 'muted'}>
            {cooldownActive
              ? m('admin.members.rename_cooldown')
              : m('admin.members.rename_available')}
          </Pill>
        </div>
      </div>
    </div>

    <div class="grid gap-4 md:grid-cols-2">
      <div class="min-w-0">
        <div class="text-sm text-muted">{m('admin.members.user_id')}</div>
        <div class="mt-1 min-w-0">
          <CopyId value={member.id} />
        </div>
      </div>
      <div>
        <div class="text-sm text-muted">{m('admin.common.joined')}</div>
        <div class="mt-1">{formatOptionalDate(member.createdAt)}</div>
      </div>
      <div class="min-w-0">
        <div class="text-sm text-muted">{m('admin.members.verified_email')}</div>
        <div class="mt-1 truncate" title={emailSummary()}>
          {emailSummary()}
        </div>
      </div>
      <div>
        <div class="text-sm text-muted">{m('admin.members.username_changes')}</div>
        <div class="mt-1">{cooldownSummary}</div>
      </div>
      <div class="min-w-0 md:col-span-2">
        <div class="text-sm text-muted">{m('admin.members.server_roles')}</div>
        <div class="mt-1 flex flex-wrap gap-1">
          {#each sortedServerRoles as roleName (roleName)}
            <Pill tone="neutral">{roleDisplayName(roleName)}</Pill>
          {/each}
          <Pill>{m('admin.members.member')}</Pill>
        </div>
      </div>
    </div>
  </div>
</Panel>
