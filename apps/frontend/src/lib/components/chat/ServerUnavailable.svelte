<!--
@component

Replaces the server chrome when this client cannot use the route's server: the
server is too old, reports an unknown version, or is unreachable. It names the
server, explains the problem, shows the server and required versions, and lets
the user run discovery again. The server layout renders it only after
discovery has a result; see `ServerInfoState.compatibilityProblem`.
-->
<script lang="ts">
  import ServerStatusView from './ServerStatusView.svelte';
  import { m } from '$lib/i18n/messages';
  import type { ServerRegistration } from '@chatto/client/server/catalog';
  import {
    MINIMUM_SUPPORTED_SERVER_VERSION,
    type ServerCompatibilityProblem
  } from '@chatto/client/server/compatibility';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { Button } from '$lib/ui/form';

  let {
    reason,
    registration,
    removable = false,
    onretry
  }: {
    /** Why the client cannot use the server. */
    reason: ServerCompatibilityProblem;
    /** Saved catalogue entry. It names the server while discovery fails. */
    registration?: Pick<ServerRegistration, 'name' | 'url' | 'iconUrl'>;
    /** Offer **Remove server**; false for the origin server. */
    removable?: boolean;
    /** Runs discovery again. A supported result makes the layout show the server. */
    onretry: () => Promise<void>;
  } = $props();

  const serverInfo = useServerScope().store.serverInfo;

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

<ServerStatusView
  {registration}
  title={copy.title}
  body={copy.body}
  tone="warning"
  testId="server-unavailable"
  {removable}
>
  {#snippet details()}
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
  {/snippet}

  {#snippet actions()}
    <Button variant="secondary" loading={checking} onclick={() => void checkAgain()}>
      {m('chat.server_unavailable.check_again')}
    </Button>
  {/snippet}
</ServerStatusView>
