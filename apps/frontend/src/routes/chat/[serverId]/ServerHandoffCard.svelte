<!-- @component Open a shared server link. An unknown server is contacted only on click. -->
<script lang="ts">
  import { onDestroy } from 'svelte';
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { getPublicServerInfo, InvalidPublicServerError } from '@chatto/client/api/server';
  import { evaluateServerCompatibility } from '@chatto/client/server/compatibility';
  import { m } from '$lib/i18n/messages';
  import { serverIdToSegment } from '$lib/navigation';
  import { serverHost } from '$lib/serverUrl';
  import { serverIdForHostname } from '$lib/navigation/serverHostname';
  import { addSignedOutServer } from '$lib/serverCatalogue';
  import { Hint } from '$lib/ui';
  import ServerLogo from '$lib/components/ServerLogo.svelte';
  import chattoIcon from '$lib/assets/chatto-icon.png';
  import { Button } from '$lib/ui/form';
  import { loadErrorMessage } from '$lib/utils/errorMessage';

  let { origin }: { origin: string | null } = $props();
  let pending = $state(false);
  let error = $state('');
  let external = $state(false);
  const lifetime = new AbortController();
  onDestroy(() => lifetime.abort());

  async function connect() {
    if (!origin || pending) return;
    pending = true;
    error = '';

    try {
      const profile = await getPublicServerInfo(origin, { signal: lifetime.signal });
      lifetime.signal.throwIfAborted();
      const saved = serverIdForHostname(new URL(origin).hostname);
      if (
        !saved &&
        evaluateServerCompatibility({ serverVersion: profile.version }).status === 'unsupported'
      ) {
        external = true;
        return;
      }
      if (!saved && !profile.authorizeUrl) {
        error = m('add_server.directory.sign_in_unavailable');
        return;
      }
      const id = saved ?? addSignedOutServer(origin, profile);
      await goto(
        resolve('/chat/[serverId]', { serverId: serverIdToSegment(id) }) + page.url.search,
        {
          replaceState: true,
          invalidateAll: true
        }
      );
    } catch (cause) {
      if (!lifetime.signal.aborted) {
        error =
          cause instanceof InvalidPublicServerError
            ? m('add_server.not_chatto_server')
            : loadErrorMessage(cause, m('add_server.connection_failed'));
      }
    } finally {
      if (!lifetime.signal.aborted) pending = false;
    }
  }
</script>

<section
  class="mx-auto w-full max-w-sm overflow-hidden rounded-2xl border border-border bg-surface shadow-md"
>
  <div
    class="flex h-28 items-center justify-center gap-4 bg-action/10 bg-[radial-gradient(circle,color-mix(in_srgb,var(--color-action)_28%,transparent)_1px,transparent_1px)] bg-size-[12px_12px] sm:h-32"
    aria-hidden="true"
  >
    <img src={chattoIcon} alt="" class="size-14 -rotate-6 rounded-xl shadow-md" />
    <span aria-hidden="true" class="iconify icon-[uil--link] text-2xl text-text"></span>
    <div class="grid size-14 rotate-6 place-items-center rounded-xl bg-surface shadow-md">
      {#if origin}
        <ServerLogo server={{ name: serverHost(origin) }} fill />
      {:else}
        <span aria-hidden="true" class="iconify icon-[uil--globe] text-3xl"></span>
      {/if}
    </div>
  </div>
  <div class="flex flex-col items-stretch gap-5 p-6 text-center sm:p-8">
    {#if origin}
      <h1
        class="text-xl leading-tight font-bold tracking-tight wrap-anywhere text-text-top sm:text-2xl"
      >
        <bdi dir="ltr">{serverHost(origin)}</bdi>
      </h1>
      {#if external}
        <Button size="lg" href={origin} opensInNewTab
          >{m('add_server.directory.open_in_new_tab')}</Button
        >
      {:else}
        {#if error}<div role="alert" class="text-start">
            <Hint tone="danger">{error}</Hint>
          </div>{/if}
        <Button
          size="lg"
          onclick={connect}
          loading={pending}
          loadingText={m('add_server.connecting')}
        >
          {error ? m('common.retry') : m('chat.server_gutter.add_server')}
        </Button>
        <p class="text-sm text-muted">{m('add_server.handoff_notice')}</p>
      {/if}
    {:else}
      <Hint tone="warning">{m('add_server.invalid_url')}</Hint>
      <Button href={resolve('/chat/servers')}>{m('add_server.directory.title')}</Button>
    {/if}
  </div>
</section>
