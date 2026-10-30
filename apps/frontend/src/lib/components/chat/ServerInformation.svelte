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
  import type { PublicServerFeature } from '@chatto/client/api/server';
  import { Button } from '$lib/ui/form';

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
    frontendFeatures = [],
    frontendVersion = '',
    frontendFeaturesLoading = false,
    frontendFeaturesError = null,
    onRetryFrontendFeatures,
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
    /** Server catalog entries that this bundled frontend release cannot use. */
    frontendFeatures?: readonly PublicServerFeature[];
    frontendVersion?: string;
    /** The independent feature catalog must not turn a failed read into success. */
    frontendFeaturesLoading?: boolean;
    frontendFeaturesError?: string | null;
    onRetryFrontendFeatures?: () => void;
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

  const serverLimited = $derived(
    compatibility.status === 'limited' || compatibility.status === 'unsupported'
  );
  const frontendLimited = $derived(
    compatibility.status !== 'unreachable' && frontendFeatures.length > 0
  );
  /** Both directions share one report. Keep existing server-feature anchors. */
  const affectedFeatures = $derived([
    ...(serverLimited
      ? features.map((feature) => ({
          id: feature,
          title: descriptions[feature].title,
          description: descriptions[feature].description,
          requirement: m('chat.compatibility.requires', {
            version: SERVER_FEATURE_REQUIREMENTS[feature]
          })
        }))
      : []),
    ...(frontendLimited
      ? frontendFeatures.map((feature) => ({
          id: `frontend-${feature.id}`,
          title: feature.name,
          description: feature.description,
          requirement: m('chat.compatibility.frontend_requires', {
            version: feature.frontendIntroducedVersion
          })
        }))
      : [])
  ]);
</script>

<div class="pane-page">
  <PaneHeader title={m('chat.server_info.title')} />
  <PaneContent>
    <div class="@container">
      <div class="grid items-start gap-6 @min-[48rem]:grid-cols-2">
        <ServerBranding
          {name}
          {iconUrl}
          {bannerUrl}
          {description}
          {welcomeMessage}
          headingLevel={2}
        />
        <div class="flex min-w-0 flex-col gap-6">
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
          <section
            id="compatibility"
            class="scroll-mt-4"
            aria-label={m('chat.compatibility.title')}
          >
            <Panel title={m('chat.compatibility.title')}>
              {#if loading}
                <LoadingDots label={m('common.loading')} />
              {:else if compatibility.status === 'supported' && !frontendLimited && !frontendFeaturesLoading && !frontendFeaturesError}
                <Hint tone="success">{m('chat.compatibility.supported')}</Hint>
              {:else if compatibility.status === 'unknown'}
                <Hint tone="warning">{m('chat.server_gutter.compatibility_unknown')}</Hint>
              {:else if compatibility.status === 'unreachable'}
                <Hint tone="warning">{m('chat.server_unavailable.unreachable_body')}</Hint>
              {/if}
              {#if !loading && frontendFeaturesLoading}
                <LoadingDots label={m('common.loading')} />
              {:else if !loading && frontendFeaturesError}
                <Hint tone="warning">
                  <p>{frontendFeaturesError}</p>
                  {#if onRetryFrontendFeatures}
                    <div class="mt-2">
                      <Button variant="secondary" onclick={onRetryFrontendFeatures}
                        >{m('common.retry')}</Button
                      >
                    </div>
                  {/if}
                </Hint>
              {/if}
              {#if !loading && (serverLimited || frontendLimited)}
                <Hint tone="warning">
                  {#if serverLimited}
                    <p class="font-medium">{m('chat.compatibility.limited')}</p>
                    {#if compatibility.status === 'unsupported'}
                      <p class="mt-2">
                        {m('chat.server_gutter.compatibility_server_too_old', {
                          version: MINIMUM_CONNECTABLE_SERVER_VERSION
                        })}
                      </p>
                    {/if}
                    <p class="mt-2">
                      {m('chat.compatibility.upgrade', {
                        version: MINIMUM_SUPPORTED_SERVER_VERSION
                      })}
                    </p>
                  {/if}
                  {#if frontendLimited}
                    <p class={['font-medium', serverLimited && 'mt-4']}>
                      {m('chat.compatibility.older_frontend')}
                    </p>
                    <p class="mt-2">
                      {m('chat.compatibility.frontend_upgrade', { version: frontendVersion })}
                    </p>
                  {/if}
                </Hint>
              {/if}
              {#if !loading && affectedFeatures.length > 0}
                <div class="mt-6 flex flex-col gap-6" data-testid="feature-limitations">
                  <h3 class="font-medium text-text">
                    {m('chat.compatibility.affected')} ({affectedFeatures.length})
                  </h3>
                  {#each affectedFeatures as feature (feature.id)}
                    <section
                      id={feature.id}
                      class="scroll-mt-4"
                      aria-labelledby={`${feature.id}-title`}
                    >
                      <h4
                        id={`${feature.id}-title`}
                        class="font-medium break-words text-text"
                        dir="auto"
                      >
                        {feature.title}
                      </h4>
                      <p class="mt-2 break-words text-muted" dir="auto">{feature.description}</p>
                      <p class="mt-2 break-words text-muted">{feature.requirement}</p>
                    </section>
                  {/each}
                </div>
              {/if}
            </Panel>
          </section>
        </div>
      </div>
    </div>
  </PaneContent>
</div>
