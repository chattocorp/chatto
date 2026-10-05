<script lang="ts">
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { serverRegistry } from '$lib/client';
  import { type RegisteredServer } from '@chatto/client/server/registry';
  import { beginOriginReauthentication } from '$lib/auth/reauth';
  import { isRemoteSignInPending, startRemoteSignIn } from '$lib/auth/remoteSignIn.svelte';
  import { TopOverlayNotice } from '$lib/ui';
  import { showsServerSignedOut } from '$lib/components/chat/serverSignedOut';
  import { m } from '$lib/i18n/messages';

  const originServer = $derived(serverRegistry.originServer);
  const originNeedsReauth = $derived(originServer?.reauthRequiredAt != null);
  const activeServer = $derived(serverRegistry.getServer(getActiveServer()));
  // Without loaded chat data, the server route itself shows the signed-out
  // view and its log-in action, so the notice would only repeat it.
  const activeRemoteNeedsReauth = $derived(
    !!activeServer &&
      activeServer.id !== originServer?.id &&
      activeServer.reauthRequiredAt != null &&
      !showsServerSignedOut(activeServer, {
        isOrigin: false,
        hasDisplayableView:
          serverRegistry.tryGetStore(activeServer.id)?.realtimeSync.hasDisplayableView ?? false
      })
  );

  const noticeServer = $derived.by<RegisteredServer | null>(() => {
    if (originNeedsReauth && originServer) return originServer;
    if (activeRemoteNeedsReauth && activeServer) return activeServer;
    return null;
  });
  const isOriginNotice = $derived(noticeServer?.id === originServer?.id);
</script>

{#if noticeServer}
  <TopOverlayNotice
    tone="warning"
    title={isOriginNotice
      ? m('ui.auth_status.origin_title')
      : m('ui.auth_status.remote_title', { server: noticeServer.name })}
    message={isOriginNotice
      ? m('ui.auth_status.origin_message')
      : m('ui.auth_status.remote_message')}
    loading={isRemoteSignInPending(noticeServer.id)}
    primaryAction={{
      label: isOriginNotice ? m('ui.auth_status.origin_action') : m('ui.auth_status.remote_action'),
      icon: 'icon-[uil--signin] rtl:-scale-x-100',
      onclick: () => {
        if (isOriginNotice) {
          beginOriginReauthentication();
          return;
        }
        void startRemoteSignIn(noticeServer);
      }
    }}
  />
{/if}
