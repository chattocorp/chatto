<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { createInviteLinkAPI, type InviteLink } from '@chatto/client/api/invitations';
  import { serverIdToSegment } from '$lib/navigation';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { createSessionGuard } from '$lib/state/server/sessionGuard.svelte';
  import { registerQueryCacheRemovalListener } from '$lib/query/cacheRegistry';
  import { adminQueryKeys } from '$lib/query/admin';
  import { queryClient } from '$lib/query/client';
  import { AccessDenied, Dialog, FormDialog, PageTitle } from '$lib/ui';
  import { Button, Checkbox, Select, TextInput } from '$lib/ui/form';
  import { m } from '$lib/i18n/messages';
  import { errorMessage, toastError } from '$lib/utils/errorMessage';
  import { toast } from '$lib/ui/toast';

  const scope = useServerScope();
  const guard = createSessionGuard(scope);
  const serverSegment = serverIdToSegment(scope.serverId);
  const canCreate = $derived(scope.store.permissions.canCreateInviteLinks);
  let maxUses = $state('1');
  let unlimitedUses = $state(false);
  let expiry = $state('7d');
  let loading = $state(false);
  let error = $state<string | null>(null);
  let invitation = $state<InviteLink | null>(null);
  const maxUsesValue = $derived(unlimitedUses ? null : Number(maxUses));
  const invalid = $derived(
    !unlimitedUses &&
      (!Number.isInteger(maxUsesValue) ||
        (maxUsesValue ?? 0) < 1 ||
        (maxUsesValue ?? 0) > 4_294_967_295)
  );

  onMount(() =>
    registerQueryCacheRemovalListener((serverId) => {
      if (serverId === scope.serverId) {
        invitation = null;
        error = null;
      }
    })
  );

  function close() {
    void goto(resolve('/chat/[serverId]/overview', { serverId: serverSegment }));
  }

  async function create() {
    if (loading || invalid || !canCreate) return;
    const snapshot = guard.snapshot();
    loading = true;
    error = null;
    try {
      const days = expiry === 'never' ? 0 : expiry === '1d' ? 1 : expiry === '30d' ? 30 : 7;
      const result = await scope.connection.getAPI(createInviteLinkAPI).create({
        maxUses: maxUsesValue,
        expiresAt: days === 0 ? null : new Date(Date.now() + days * 86_400_000).toISOString()
      });
      if (!guard.isCurrent(snapshot)) return;
      invitation = result;
      await queryClient.invalidateQueries({
        queryKey: adminQueryKeys.invitations(scope.serverId, scope.connection)
      });
    } catch (cause) {
      if (guard.isCurrent(snapshot)) error = errorMessage(cause);
    } finally {
      loading = false;
    }
  }

  async function copy() {
    if (!invitation) return;
    try {
      await navigator.clipboard.writeText(invitation.link);
      toast.success(m('invite_links.copied'));
    } catch {
      toastError(undefined, m('room.message.actions.copy_link_failed'));
    }
  }
</script>

<PageTitle title={m('invite_links.create')} />

{#if invitation}
  <Dialog visible title={m('invite_links.created')} onclose={close}>
    <div class="flex flex-col gap-4">
      <code class="overflow-x-auto surface-box p-3 text-sm whitespace-nowrap select-all" dir="ltr"
        >{invitation.link}</code
      >
      <Button onclick={copy}
        ><span class="iconify icon-[uil--copy]" aria-hidden="true"></span>{m(
          'invite_links.copy'
        )}</Button
      >
      <a
        class="link"
        href={resolve('/chat/[serverId]/manage/server/invite-links', { serverId: serverSegment })}
        >{m('invite_links.own')}</a
      >
    </div>
  </Dialog>
{:else if canCreate}
  <FormDialog
    visible
    title={m('invite_links.create')}
    submitLabel={m('invite_links.create')}
    submitIcon="iconify icon-[uil--plus]"
    {loading}
    disabled={invalid}
    {error}
    onsubmit={create}
    onclose={close}
  >
    <div class="flex flex-col gap-4">
      <TextInput
        id="invitation-max-uses"
        label={m('invite_links.max_uses')}
        bind:value={maxUses}
        disabled={unlimitedUses || loading}
        error={invalid ? m('invite_links.max_uses_error') : undefined}
      />
      <Checkbox
        id="invitation-unlimited-uses"
        bind:checked={unlimitedUses}
        label={m('invite_links.unlimited_uses')}
        disabled={loading}
      />
      <Select
        id="invitation-expiry"
        label={m('invite_links.expiry')}
        bind:value={expiry}
        disabled={loading}
        options={[
          { value: '1d', label: m('invite_links.expiry_1d') },
          { value: '7d', label: m('invite_links.expiry_7d') },
          { value: '30d', label: m('invite_links.expiry_30d') },
          { value: 'never', label: m('invite_links.expiry_never') }
        ]}
      />
      <a
        class="link"
        href={resolve('/chat/[serverId]/manage/server/invite-links', { serverId: serverSegment })}
        >{m('invite_links.own')}</a
      >
    </div>
  </FormDialog>
{:else if scope.store.permissions.loaded}
  <AccessDenied
    message={m('ui.access_denied.message')}
    backHref={resolve('/chat/[serverId]/overview', { serverId: serverSegment })}
    backLabel={m('chat.management_nav.back_to_server')}
  />
{/if}
