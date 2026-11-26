<!-- @component Open a shared server link. An unknown server is contacted only on click. -->
<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { getPublicServerInfo, InvalidPublicServerError } from '@chatto/client/api/server';
  import { evaluateServerCompatibility } from '@chatto/client/server/compatibility';
  import { m } from '$lib/i18n/messages';
  import { serverIdToSegment } from '$lib/navigation';
  import { serverIdForHostname } from '$lib/navigation/serverHostname';
  import { addSignedOutServer, findServerByUrl } from '$lib/serverCatalogue';
  import { Hint, PageTitle, PaneContent } from '$lib/ui';
  import { Button } from '$lib/ui/form';
  import { loadErrorMessage } from '$lib/utils/errorMessage';
  import type { PageProps } from './$types';

  let { data }: PageProps = $props();
  let pending = $state(false);
  let error = $state('');
  let external = $state(false);
  let controller: AbortController | undefined;

  /** Drop unfinished discovery and feedback when the link changes or the page closes. */
  function linkLifetime() {
    return () => {
      controller?.abort();
      pending = false;
      external = false;
      error = '';
    };
  }

  async function connect(hostname: string) {
    if (pending) return;
    const request = new AbortController();
    controller = request;
    pending = true;
    error = '';
    const origin = `https://${hostname}`;
    try {
      const profile = await getPublicServerInfo(origin, { signal: request.signal });
      request.signal.throwIfAborted();
      const saved = findServerByUrl(origin);
      const owner = serverIdForHostname(hostname);
      if (
        (owner && owner !== saved?.id) ||
        (!saved &&
          evaluateServerCompatibility({ serverVersion: profile.version }).status === 'unsupported')
      ) {
        external = true;
        return;
      }
      if (!saved && !profile.authorizeUrl) {
        error = m('add_server.directory.sign_in_unavailable');
        return;
      }
      const id = saved?.id ?? addSignedOutServer(origin, profile);
      await goto(resolve('/chat/[serverId]', { serverId: serverIdToSegment(id) }), {
        replaceState: true
      });
    } catch (cause) {
      if (!request.signal.aborted) {
        error =
          cause instanceof InvalidPublicServerError
            ? m('add_server.not_chatto_server')
            : loadErrorMessage(cause, m('add_server.connection_failed'));
      }
    } finally {
      if (!request.signal.aborted) pending = false;
    }
  }
</script>

<PageTitle title={m('add_server.directory.connect_by_address')} />
<PaneContent>
  {#key data.hostname}
    <section
      class="mx-auto flex w-full max-w-md flex-col items-start gap-4 py-8"
      {@attach linkLifetime}
    >
      {#if data.hostname}
        {@const hostname = data.hostname}
        <h1 class="text-2xl font-semibold wrap-anywhere text-text-top">
          <bdi dir="ltr">{hostname}</bdi>
        </h1>
        {#if external || data.external}
          <Button href={`https://${hostname}`} opensInNewTab
            >{m('add_server.directory.open_in_new_tab')}</Button
          >
        {:else}
          <p>{m('add_server.handoff_notice')}</p>
          {#if error}<div role="alert"><Hint tone="danger">{error}</Hint></div>{/if}
          <Button
            onclick={() => connect(hostname)}
            loading={pending}
            loadingText={m('add_server.connecting')}
          >
            {error ? m('common.retry') : m('add_server.directory.connect_by_address')}
          </Button>
        {/if}
      {:else}
        <Hint tone="warning">{m('add_server.invalid_url')}</Hint>
        <Button href={resolve('/chat/servers')}>{m('add_server.directory.title')}</Button>
      {/if}
    </section>
  {/key}
</PaneContent>
