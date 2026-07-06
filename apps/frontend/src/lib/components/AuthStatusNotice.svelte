<script lang="ts">
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { serverRegistry, type RegisteredServer } from '$lib/state/server/registry.svelte';
  import { beginOriginReauthentication, startRemoteReauthentication } from '$lib/auth/reauth';
  import { TopOverlayNotice } from '$lib/ui';
  import { toast } from '$lib/ui/toast';
  import * as m from '$lib/i18n/messages';

  let reconnectingServerId = $state<string | null>(null);

  const originServer = $derived(serverRegistry.originServer);
  const originNeedsReauth = $derived(originServer?.reauthRequiredAt != null);
  const activeServer = $derived(serverRegistry.getServer(getActiveServer()));
  const activeRemoteNeedsReauth = $derived(
    !!activeServer &&
      activeServer.id !== originServer?.id &&
      activeServer.reauthRequiredAt != null
  );

  const noticeServer = $derived.by<RegisteredServer | null>(() => {
    if (originNeedsReauth && originServer) return originServer;
    if (activeRemoteNeedsReauth && activeServer) return activeServer;
    return null;
  });
  const isOriginNotice = $derived(noticeServer?.id === originServer?.id);

  async function reconnectRemote(server: RegisteredServer) {
    reconnectingServerId = server.id;
    try {
      await startRemoteReauthentication(server);
    } catch (err) {
      reconnectingServerId = null;
      toast.error(err instanceof Error ? err.message : m['ui.auth_status.remote_failed']());
    }
  }
</script>

{#if noticeServer}
  <TopOverlayNotice
    tone="warning"
    title={isOriginNotice
      ? m['ui.auth_status.origin_title']()
      : m['ui.auth_status.remote_title']({ server: noticeServer.name })}
    message={isOriginNotice
      ? m['ui.auth_status.origin_message']()
      : m['ui.auth_status.remote_message']()}
    loading={reconnectingServerId === noticeServer.id}
    primaryAction={{
      label: isOriginNotice
        ? m['ui.auth_status.origin_action']()
        : m['ui.auth_status.remote_action'](),
      icon: 'uil--signin',
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
