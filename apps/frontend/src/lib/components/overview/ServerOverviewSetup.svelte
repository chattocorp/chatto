<!--
@component

A reminder for administrators to complete the public server profile. It lists
the empty profile fields and links to the General settings. The owner decides
who sees it. The viewer can hide it on this device. Renders nothing when the
profile is complete.
-->
<script lang="ts">
  import { resolve } from '$app/paths';
  import { m } from '$lib/i18n/messages';
  import { getReactiveLocale } from '$lib/i18n/state.svelte';
  import type { ProfileSetupItem } from '$lib/serverOverview';
  import { dismissOverviewCard, isOverviewCardDismissed } from '$lib/storage/overviewDismissals';
  import { CompactActionButton, Hint } from '$lib/ui';

  let {
    serverId,
    serverSegment,
    missing
  }: {
    serverId: string;
    serverSegment: string;
    /** Empty profile fields, from `missingProfileItems`. */
    missing: readonly ProfileSetupItem[];
  } = $props();

  let dismissedNow = $state(false);
  const visible = $derived(
    missing.length > 0 && !dismissedNow && !isOverviewCardDismissed(serverId, 'setup')
  );

  const labels: Record<ProfileSetupItem, () => string> = {
    logo: () => m('chat.overview.setup.items.logo'),
    banner: () => m('chat.overview.setup.items.banner'),
    description: () => m('chat.overview.setup.items.description'),
    welcome_message: () => m('chat.overview.setup.items.welcome_message')
  };

  function dismiss() {
    dismissOverviewCard(serverId, 'setup');
    dismissedNow = true;
  }
</script>

{#if visible}
  <div data-testid="server-overview-setup">
    <Hint tone="warning" icon="icon-[uil--wrench]">
      <div class="flex flex-wrap items-center gap-x-4 gap-y-2">
        <div class="flex min-w-0 flex-1 flex-col gap-1">
          <h2 class="font-semibold text-text-top">{m('chat.overview.setup.title')}</h2>
          <p class="text-text">
            {m('chat.overview.setup.missing', {
              items: new Intl.ListFormat(getReactiveLocale(), { type: 'conjunction' }).format(
                missing.map((item) => labels[item]())
              )
            })}
          </p>
        </div>
        <a
          class="font-medium link"
          href={resolve('/chat/[serverId]/manage/server/general', { serverId: serverSegment })}
        >
          {m('chat.overview.setup.edit_profile')}
        </a>
        <CompactActionButton label={m('chat.overview.setup.hide')} onclick={dismiss}>
          <span aria-hidden="true" class="iconify icon-[uil--times]"></span>
        </CompactActionButton>
      </div>
    </Hint>
  </div>
{/if}
