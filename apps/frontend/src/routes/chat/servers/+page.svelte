<!--
@component

The shared Server Directory and standalone start page. The Server Gutter
opens this page in every host.
-->
<script lang="ts">
  import ServerDirectory from '$lib/components/ServerDirectory.svelte';
  import { knownServers, serverRegistry } from '$lib/client';
  import { m } from '$lib/i18n/messages';
  import { PageTitle, PaneContent, PaneHeader, ScrollFader } from '$lib/ui';
  import { Button } from '$lib/ui/form';
  import { page } from '$app/state';
  import { pushState } from '$app/navigation';

  const lookupRequested = $derived(page.state.modal?.type === 'serverAddress');
  const hasServers = $derived(serverRegistry.servers.length > 0 || knownServers.servers.length > 0);

  function closeLookup() {
    if (page.state.modal?.type === 'serverAddress') history.back();
  }
</script>

<PageTitle
  title={hasServers ? m('add_server.directory.title') : m('auth.login.welcome_page_title')}
/>

<div class="relative isolate pane-page">
  {#if hasServers}
    {#snippet addressAction()}
      <Button
        variant="secondary"
        onclick={() => pushState('', { modal: { type: 'serverAddress' } })}
      >
        <span class="iconify icon-[uil--globe]" aria-hidden="true"></span>
        <span>{m('add_server.directory.connect_by_address')}</span>
      </Button>
    {/snippet}
    <PaneHeader title={m('add_server.directory.title')} actions={addressAction} collapseActions />
    <PaneContent fluid>
      <ServerDirectory {lookupRequested} onLookupClose={closeLookup} />
    </PaneContent>
  {:else}
    <!-- Decorative layers follow viewport geometry without resize listeners.
         Reduced motion keeps the companion at a fixed offset. -->
    <div class="pointer-events-none absolute inset-0 -z-10 overflow-hidden" aria-hidden="true">
      <span
        class="iconify absolute end-6 top-6 icon-[uil--comments-alt] -rotate-12 text-6xl text-action opacity-50 sm:text-9xl"
        aria-hidden="true"
      ></span>
      <span
        class="iconify absolute end-[calc(-8rem+4vw)] top-[calc(-8rem+6vh)] icon-[uil--comments-alt] rotate-12 text-[clamp(24rem,55vw,60rem)] text-muted opacity-5 motion-reduce:-end-24 motion-reduce:-top-24"
        aria-hidden="true"
      ></span>
    </div>
    <ScrollFader top bottom>
      <div
        class="flex flex-1 items-center justify-center px-4 pt-24 pb-12 sm:p-12"
        data-page-reveal
      >
        <ServerDirectory />
      </div>
    </ScrollFader>
  {/if}
</div>
