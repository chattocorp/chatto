<!-- @component A new destination owns its consent state and discovery request. -->
<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import { CenteredPage, PageTitle } from '$lib/ui';
  import ServerHandoffCard from './ServerHandoffCard.svelte';
  import type { PageProps } from './$types';

  let { data }: PageProps = $props();
</script>

<PageTitle title={m('chat.server_gutter.add_server')} />
<CenteredPage>
  {#key data.origin}
    <ServerHandoffCard origin={data.origin} />
  {/key}
</CenteredPage>
