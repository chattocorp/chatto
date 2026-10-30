<script lang="ts">
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import ServerInformation from '$lib/components/chat/ServerInformation.svelte';
  import { serverHost } from '$lib/serverUrl';
  import { PageTitle } from '$lib/ui';
  import { m } from '$lib/i18n/messages';
  import { version as frontendVersion } from '../../../../../package.json';
  import { listServerFeatures } from '@chatto/client/api/server';
  import {
    limitedFrontendFeatures,
    normalizeServerVersion
  } from '@chatto/client/server/compatibility';
  import { createQuery } from '$lib/query/client';
  import { serverQueryRoot } from '$lib/query/keys';
  import { getReactiveLocale } from '$lib/i18n/state.svelte';
  import { errorMessage } from '$lib/utils/errorMessage';
  const { store, connection, serverId } = useServerScope();
  const info = store.serverInfo;
  const frontendRelease = normalizeServerVersion(frontendVersion) ?? undefined;
  const catalogEnabled = $derived(
    !info.isOriginServer && info.lastDiscoveredAt !== null && info.error === null
  );
  // The page owns this public snapshot. New server versions and locales use
  // separate cache entries; requests stop when the page no longer needs them.
  const catalog = createQuery(() => {
    const locale = getReactiveLocale();
    return {
      queryKey: [
        ...serverQueryRoot(serverId),
        'public-features',
        info.version,
        locale,
        frontendRelease
      ],
      queryFn: ({ signal }) =>
        listServerFeatures(connection.connectBaseUrl, {
          locale,
          introducedAfterFrontendVersion: frontendRelease,
          signal
        }),
      enabled: catalogEnabled,
      staleTime: Infinity
    };
  });
</script>

<PageTitle title={m('chat.server_info.title')} />
<ServerInformation
  name={info.name}
  iconUrl={info.iconUrl}
  bannerUrl={info.bannerUrl}
  description={info.description}
  welcomeMessage={info.welcomeMessage}
  host={serverHost(connection.connectBaseUrl)}
  version={info.version}
  compatibility={info.compatibility}
  features={info.limitedFeatures}
  frontendFeatures={catalogEnabled
    ? limitedFrontendFeatures(catalog.data ?? [], frontendVersion)
    : []}
  frontendFeaturesLoading={catalogEnabled && catalog.isPending}
  frontendFeaturesError={catalogEnabled && catalog.error
    ? errorMessage(catalog.error, m('common.error.generic'))
    : null}
  onRetryFrontendFeatures={() => void catalog.refetch()}
  {frontendVersion}
  loading={info.loading && info.lastDiscoveredAt === null}
/>
