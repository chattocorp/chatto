<!--
@component

Replaces the server chrome with a centred explanation when the client cannot
show a server. It names the server with its logo and host, shows a title and
a body text, and renders the caller's details and actions below them. When
the caller allows it, it also offers **Remove server**, which opens the same
confirmation as the gutter menu. `ServerUnavailable` and
`ServerSignedOut` use it.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import { pushState } from '$app/navigation';
  import { m } from '$lib/i18n/messages';
  import ServerLogo from '$lib/components/ServerLogo.svelte';
  import type { ServerRegistration } from '@chatto/client/server/catalog';
  import { serverDisplayName } from '@chatto/client/server/state';
  import { serverHost } from '$lib/serverUrl';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { PageTitle } from '$lib/ui';
  import { Button } from '$lib/ui/form';

  let {
    registration,
    title,
    body,
    tone,
    testId,
    removable = false,
    details,
    actions
  }: {
    /** Saved catalogue entry. It names the server while discovery fails. */
    registration?: Pick<ServerRegistration, 'name' | 'url' | 'iconUrl'>;
    /** Heading that states the server's state. */
    title: string;
    /** Explanation below the heading. */
    body: string;
    /** `warning` shows the heading with a warning icon and colour. */
    tone?: 'warning';
    /** `data-testid` of the root element. */
    testId: string;
    /**
     * Offer **Remove server**. The origin server belongs to the deployment, so
     * callers pass `false` for it.
     */
    removable?: boolean;
    /** Details below the explanation, such as version facts. */
    details?: Snippet;
    /** Actions for the server's state, before **Remove server**. */
    actions: Snippet;
  } = $props();

  const titleId = $props.id();
  const { serverId, store } = useServerScope();
  const serverInfo = store.serverInfo;

  const serverName = $derived(serverDisplayName(serverInfo, registration?.name));
  const host = $derived(registration ? serverHost(registration.url) : null);
  const logoUrl = $derived(serverInfo.iconUrl ?? registration?.iconUrl ?? null);
  function removeServer(): void {
    pushState('', { modal: { type: 'removeServer', serverId, spaceName: serverName } });
  }
</script>

<PageTitle title={serverName} />

<div class="pane-page overflow-y-auto" data-testid={testId}>
  <section
    class="m-auto flex w-full max-w-md flex-col items-center gap-5 p-6 text-center"
    aria-labelledby={titleId}
  >
    <div class="h-16 w-16">
      <ServerLogo server={{ name: serverName, logoUrl }} fill />
    </div>
    <div class="max-w-full min-w-0">
      <p class="truncate font-medium text-text"><bdi>{serverName}</bdi></p>
      {#if host}
        <p class="truncate text-sm text-muted" dir="ltr">{host}</p>
      {/if}
    </div>

    <div class="flex flex-col gap-2">
      <h1
        id={titleId}
        class={[
          'flex items-center justify-center gap-2 text-lg font-semibold',
          tone === 'warning' && 'text-warning'
        ]}
      >
        {#if tone === 'warning'}
          <span class="iconify icon-[uil--exclamation-circle] shrink-0" aria-hidden="true"></span>
        {/if}
        {title}
      </h1>
      <p class="text-muted">{body}</p>
    </div>

    {@render details?.()}

    <div class="flex flex-wrap justify-center gap-2">
      {@render actions()}
      {#if removable}
        <Button variant="danger" onclick={removeServer}>
          {m('room_list.remove_server')}
        </Button>
      {/if}
    </div>
  </section>
</div>
