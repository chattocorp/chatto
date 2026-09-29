<!--
@component

Replaces the server chrome when this client cannot use the route's server: the
server is too old, reports an unknown version, or is unreachable. It names the
server, explains the problem, shows the server and required versions, and lets
the user run discovery again. The server layout renders it only after
discovery has a result; see `ServerInfoState.compatibilityProblem`.
-->
<script lang="ts">
  import ServerLogo from '$lib/components/ServerLogo.svelte';
  import { m } from '$lib/i18n/messages';
  import type { ServerRegistration } from '$lib/state/server/catalog.svelte';
  import {
    MINIMUM_SUPPORTED_SERVER_VERSION,
    type ServerCompatibilityProblem
  } from '$lib/state/server/compatibility';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { PageTitle } from '$lib/ui';
  import { Button } from '$lib/ui/form';

  let {
    reason,
    registration,
    onretry
  }: {
    /** Why the client cannot use the server. */
    reason: ServerCompatibilityProblem;
    /** Saved catalogue entry. It names the server while discovery fails. */
    registration?: Pick<ServerRegistration, 'name' | 'url' | 'iconUrl'>;
    /** Runs discovery again. A supported result makes the layout show the server. */
    onretry: () => Promise<void>;
  } = $props();

  const { store } = useServerScope();
  const serverInfo = store.serverInfo;

  // Discovery keeps the default name until it succeeds. Prefer the name that
  // was saved at registration over that default.
  const serverName = $derived(
    serverInfo.name !== 'Chatto' ? serverInfo.name : (registration?.name ?? serverInfo.name)
  );
  const serverHost = $derived.by(() => {
    if (!registration) return null;
    try {
      return new URL(registration.url).host;
    } catch {
      return registration.url;
    }
  });
  const logoUrl = $derived(serverInfo.iconUrl ?? registration?.iconUrl ?? null);

  const copy = $derived.by(() => {
    switch (reason) {
      case 'server-too-old':
        return {
          title: m('chat.server_unavailable.too_old_title'),
          body: m('chat.server_unavailable.too_old_body')
        };
      case 'server-version-unknown':
        return {
          title: m('chat.server_unavailable.unknown_title'),
          body: m('chat.server_unavailable.unknown_body')
        };
      case 'unreachable':
        return {
          title: m('chat.server_gutter.unreachable'),
          body: m('chat.server_unavailable.unreachable_body')
        };
    }
  });

  let checking = $state(false);

  async function checkAgain(): Promise<void> {
    checking = true;
    try {
      await onretry();
    } finally {
      checking = false;
    }
  }
</script>

<PageTitle title={serverName} />

<div class="pane-page overflow-y-auto" data-testid="server-unavailable">
  <section
    class="m-auto flex w-full max-w-md flex-col items-center gap-5 p-6 text-center"
    aria-labelledby="server-unavailable-title"
  >
    <div class="h-16 w-16">
      <ServerLogo server={{ name: serverName, logoUrl }} fill />
    </div>
    <div class="max-w-full min-w-0">
      <p class="truncate font-medium text-text"><bdi>{serverName}</bdi></p>
      {#if serverHost}
        <p class="truncate text-sm text-muted" dir="ltr">{serverHost}</p>
      {/if}
    </div>

    <div class="flex flex-col gap-2">
      <h1
        id="server-unavailable-title"
        class="flex items-center justify-center gap-2 text-lg font-semibold text-warning"
      >
        <span class="iconify icon-[uil--exclamation-circle] shrink-0" aria-hidden="true"></span>
        {copy.title}
      </h1>
      <p class="text-muted">{copy.body}</p>
    </div>

    {#if reason !== 'unreachable'}
      <dl class="grid grid-cols-2 gap-x-4 gap-y-1 text-sm">
        {#if serverInfo.version}
          <dt class="text-end text-muted">{m('chat.server_unavailable.server_version')}</dt>
          <dd class="text-start font-medium" data-testid="server-unavailable-version">
            {serverInfo.version}
          </dd>
        {/if}
        <dt class="text-end text-muted">{m('chat.server_unavailable.required_version')}</dt>
        <dd class="text-start font-medium" data-testid="server-unavailable-required-version">
          {m('chat.server_unavailable.required_version_value', {
            version: MINIMUM_SUPPORTED_SERVER_VERSION
          })}
        </dd>
      </dl>
    {/if}

    <Button variant="secondary" loading={checking} onclick={() => void checkAgain()}>
      {m('chat.server_unavailable.check_again')}
    </Button>
  </section>
</div>
