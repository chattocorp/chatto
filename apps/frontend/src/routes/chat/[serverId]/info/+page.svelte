<script lang="ts">
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import ServerInformation from '$lib/components/chat/ServerInformation.svelte';
  import { serverHost } from '$lib/serverUrl';
  import { PageTitle } from '$lib/ui';
  import { m } from '$lib/i18n/messages';
  const { store, connection } = useServerScope();
  const info = store.serverInfo;
</script>

<PageTitle title={m('chat.server_info.title')} />
<ServerInformation
  name={info.name}
  host={serverHost(connection.connectBaseUrl)}
  version={info.version}
  compatibility={info.compatibility}
  features={info.limitedFeatures}
  loading={info.loading && info.lastDiscoveredAt === null}
/>
