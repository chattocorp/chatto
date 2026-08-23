<script lang="ts">
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { serverRegistry, type RegisteredServer } from '$lib/state/server/registry.svelte';
  import { beginOriginReauthentication, startRemoteReauthentication } from '$lib/auth/reauth';
  import Deadline from '$lib/lifecycle/Deadline.svelte';
  import { TopOverlayNotice } from '$lib/ui';
  import { toast } from '$lib/ui/toast';
  import { m } from '$lib/i18n/messages';

  let reconnectingServerId = $state<string | null>(null);
  let deadlineRevision = $state(0);

  const expiryWarningLeadMs = 7 * 24 * 60 * 60 * 1000;

  const originServer = $derived(serverRegistry.originServer);
  const originNeedsReauth = $derived(originServer?.reauthRequiredAt != null);
  const activeServer = $derived(serverRegistry.getServer(getActiveServer()));
  const activeRemoteNeedsReauth = $derived(
    !!activeServer && activeServer.id !== originServer?.id && activeServer.reauthRequiredAt != null
  );
  const activeRenewableSessionExpiry = $derived(
    activeServer?.refreshToken && activeServer.refreshTokenExpiresAt
      ? activeServer.refreshTokenExpiresAt
      : null
  );
  const activeRenewableSessionExpiresSoon = $derived.by(() => {
    void deadlineRevision;
    if (activeRenewableSessionExpiry === null) return false;
    const remaining = activeRenewableSessionExpiry - Date.now();
    return remaining > 0 && remaining <= expiryWarningLeadMs;
  });

  const noticeServer = $derived.by<RegisteredServer | null>(() => {
    if (originNeedsReauth && originServer) return originServer;
    if (activeRemoteNeedsReauth && activeServer) return activeServer;
    if (activeRenewableSessionExpiresSoon && activeServer) return activeServer;
    return null;
  });
  const isOriginNotice = $derived(noticeServer?.id === originServer?.id);
  const isExpiryNotice = $derived(
    !!noticeServer &&
      noticeServer.id === activeServer?.id &&
      !originNeedsReauth &&
      !activeRemoteNeedsReauth &&
      activeRenewableSessionExpiresSoon
  );

  async function reconnectRemote(server: RegisteredServer) {
    reconnectingServerId = server.id;
    try {
      await startRemoteReauthentication(server);
    } catch {
      reconnectingServerId = null;
      toast.error(m('ui.auth_status.remote_failed'));
    }
  }
</script>

{#if activeRenewableSessionExpiry !== null}
  <Deadline
    at={activeRenewableSessionExpiry}
    offsetMilliseconds={-expiryWarningLeadMs}
    onreached={() => deadlineRevision++}
  />
{/if}

{#if noticeServer}
  <TopOverlayNotice
    tone="warning"
    title={isExpiryNotice
      ? m('ui.auth_status.expiry_title', { server: noticeServer.name })
      : isOriginNotice
        ? m('ui.auth_status.origin_title')
        : m('ui.auth_status.remote_title', { server: noticeServer.name })}
    message={isExpiryNotice
      ? m('ui.auth_status.expiry_message')
      : isOriginNotice
        ? m('ui.auth_status.origin_message')
        : m('ui.auth_status.remote_message')}
    loading={reconnectingServerId === noticeServer.id}
    primaryAction={{
      label: isExpiryNotice
        ? m('ui.auth_status.expiry_action')
        : isOriginNotice
          ? m('ui.auth_status.origin_action')
          : m('ui.auth_status.remote_action'),
      icon: 'icon-[uil--signin]',
      onclick: () => {
        if (isOriginNotice) {
          beginOriginReauthentication();
          return;
        }
        void reconnectRemote(noticeServer);
      }
    }}
  />
{/if}
