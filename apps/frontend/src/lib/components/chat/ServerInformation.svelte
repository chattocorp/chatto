<!-- @component Public server details and compatibility. The route supplies discovery facts without requiring a chat session. -->
<script lang="ts">
  import {
    MINIMUM_SUPPORTED_SERVER_VERSION,
    MINIMUM_CONNECTABLE_SERVER_VERSION,
    SERVER_FEATURE_REQUIREMENTS,
    type ServerCompatibilityResult,
    type ServerFeature
  } from '@chatto/client/server/compatibility';
  import { m } from '$lib/i18n/messages';
  import { CompactActionButton, Hint, LoadingDots, PaneContent, PaneHeader, Panel } from '$lib/ui';
  import { copyToClipboard } from '$lib/utils/copyToClipboard';
  import ServerBranding from '$lib/components/ServerBranding.svelte';

  let {
    name,
    iconUrl = null,
    bannerUrl = null,
    description = null,
    welcomeMessage = null,
    host,
    version,
    compatibility,
    features,
    loading = false
  }: {
    name: string;
    iconUrl?: string | null;
    bannerUrl?: string | null;
    description?: string | null;
    welcomeMessage?: string | null;
    /** Configured hostname, including a non-default port. Never a full request URL. */
    host: string;
    version: string;
    compatibility: ServerCompatibilityResult;
    features: readonly ServerFeature[];
    loading?: boolean;
  } = $props();
  const descriptions = $derived({
    threadViews: {
      title: m('chat.compatibility.thread_views_title'),
      description: m('chat.compatibility.thread_views_description')
    },
    independentSlowMode: {
      title: m('chat.compatibility.independent_slow_mode_title'),
      description: m('chat.compatibility.independent_slow_mode_description')
    }
  } satisfies Record<ServerFeature, { title: string; description: string }>);
</script>

<div class="pane-page">
  <PaneHeader title={m('chat.server_info.title')} />
  <PaneContent>
    <div class="flex flex-col gap-6">
      <Panel>
        <ServerBranding
          {name}
          {iconUrl}
          {bannerUrl}
          {description}
          {welcomeMessage}
          headingLevel={2}
        />
      </Panel>
      <Panel title={m('chat.server_info.details')}>
        <div class="flex items-center gap-2">
          <span class="min-w-0 break-all text-muted">{host}</span>
          <CompactActionButton
            label={m('room_list.copy_server_hostname')}
            onclick={() => void copyToClipboard(host)}
            touchFriendly
          >
            <span class="iconify icon-[uil--copy]" aria-hidden="true"></span>
          </CompactActionButton>
        </div>
        {#if version}
          <p class="mt-2 text-muted">{m('chat.server_gutter.version', { version })}</p>
        {/if}
      </Panel>
      <section id="compatibility" class="scroll-mt-4" aria-label={m('chat.compatibility.title')}>
        <Panel title={m('chat.compatibility.title')}>
          {#if loading}
            <LoadingDots label={m('common.loading')} />
          {:else if compatibility.status === 'supported'}
            <Hint tone="success">{m('chat.compatibility.supported')}</Hint>
          {:else if compatibility.status === 'unknown'}
            <Hint tone="warning">{m('chat.server_gutter.compatibility_unknown')}</Hint>
          {:else if compatibility.status === 'unreachable'}
            <Hint tone="warning">{m('chat.server_unavailable.unreachable_body')}</Hint>
          {:else}
            <Hint tone="warning">
              <p class="font-medium">{m('chat.compatibility.limited')}</p>
              {#if compatibility.status === 'unsupported'}
                <p class="mt-2">
                  {m('chat.server_gutter.compatibility_server_too_old', {
                    version: MINIMUM_CONNECTABLE_SERVER_VERSION
                  })}
                </p>
              {/if}
              <p class="mt-2">
                {m('chat.compatibility.upgrade', { version: MINIMUM_SUPPORTED_SERVER_VERSION })}
              </p>
            </Hint>
          {/if}
          {#if !loading && features.length > 0 && (compatibility.status === 'limited' || compatibility.status === 'unsupported')}
            <div class="mt-6 flex flex-col gap-6">
              <h3 class="font-medium text-text">
                {m('chat.compatibility.affected')} ({features.length})
              </h3>
              {#each features as feature (feature)}
                <section id={feature} class="scroll-mt-4" aria-labelledby={`${feature}-title`}>
                  <h4 id={`${feature}-title`} class="font-medium text-text">
                    {descriptions[feature].title}
                  </h4>
                  <p class="mt-2 text-muted">{descriptions[feature].description}</p>
                  <p class="mt-2 text-muted">
                    {m('chat.compatibility.requires', {
                      version: SERVER_FEATURE_REQUIREMENTS[feature]
                    })}
                  </p>
                </section>
              {/each}
            </div>
          {/if}
        </Panel>
      </section>
    </div>
  </PaneContent>
</div>
