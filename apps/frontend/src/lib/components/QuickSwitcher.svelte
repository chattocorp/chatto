<script lang="ts">
  import AccountName from '$lib/components/users/AccountName.svelte';
  import AccountNameTokens from '$lib/components/users/AccountNameTokens.svelte';
  import DirectMessageName from '$lib/components/users/DirectMessageName.svelte';
  import { untrack } from 'svelte';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import { m } from '$lib/i18n/messages';
  import { LoadingFog, MenuSection, PaletteSearch, PaletteDialog } from '$lib/ui';
  import { quickSwitcher } from '$lib/state/globals.svelte';
  import { getGradientForName } from '$lib/utils/gradients';
  import { QuickSwitcherModel, type QuickSwitcherAvatarUser } from './quickSwitcherModel.svelte';

  let {
    onNextUnread
  }: {
    onNextUnread?: () => Promise<void>;
  } = $props();
  const model = new QuickSwitcherModel(() => onNextUnread?.());
  let resultsEl: HTMLElement | undefined;

  function activateResults(node: HTMLElement) {
    resultsEl = node;
    // Activation resets this opening's query; its internal reads are not dependencies.
    untrack(() => model.activate());
    return () => model.deactivate();
  }

  // Rebuild from the canonical per-server room stores while the switcher is open.
  $effect(() => model.syncCatalog());
  // Fence transient message plaintext at every server's search privacy boundary.
  $effect(() => model.syncPrivacy());

  function handleKeydown(event: KeyboardEvent) {
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault();
      model.moveSelection(event.key === 'ArrowDown' ? 1 : -1);
      scrollSelectedIntoView();
    } else if (event.key === 'Enter') {
      event.preventDefault();
      model.selectCurrent();
    }
  }

  function scrollSelectedIntoView() {
    requestAnimationFrame(() => {
      const selected = resultsEl?.querySelector(`[data-index="${model.selectedIndex}"]`);
      selected?.scrollIntoView({ block: 'nearest' });
    });
  }
</script>

{#snippet avatar(user: QuickSwitcherAvatarUser)}
  <UserAvatar {user} size="xs" useLiveProfile={false} />
{/snippet}

<PaletteDialog
  visible={quickSwitcher.visible}
  onclose={() => quickSwitcher.close()}
  title={m('quick_switcher.title')}
  class="quick-switcher"
>
  <PaletteSearch
    class="shrink-0 max-md:border-b max-md:border-border"
    value={model.query}
    onvaluechange={(value) => model.setQuery(value)}
    onkeydown={handleKeydown}
    label={m('quick_switcher.placeholder')}
    loading={model.loading}
    focusOnMount
  >
    <kbd class="keycap hidden md:inline-flex">Esc</kbd>
  </PaletteSearch>

  <MenuSection
    class="max-h-[min(20rem,55dvh)] min-h-0 overflow-y-auto max-md:max-h-none max-md:flex-1"
  >
    <nav {@attach activateResults} class="sidebar-nav">
      {#if model.loading && model.filtered.length === 0}
        <LoadingFog class="m-2 h-24" />
      {/if}
      {#if model.filtered.length === 0 && !model.loading}
        <p class="px-3 py-6 text-center text-muted">
          {model.query.trim() === '?'
            ? m('quick_switcher.message_search.prompt')
            : model.query.trim().startsWith('?')
              ? m('quick_switcher.message_search.no_results')
              : m('quick_switcher.no_results')}
        </p>
      {:else}
        {#each model.filtered as item, index (`${item.serverId}:${item.kind}:${item.id}`)}
          {@const header = model.groupHeader(index)}

          {#if header}
            <div class="command-palette-heading">
              {header}
            </div>
          {/if}

          <button
            data-index={index}
            type="button"
            class={[
              'command-palette-result',
              item.kind === 'message' ? 'items-start py-2' : '',
              index === model.selectedIndex ? 'command-palette-result-active' : ''
            ]}
            onclick={() => model.select(item)}
            onpointermove={() => model.selectIndex(index)}
          >
            {#if item.kind === 'message'}
              <span class="command-palette-leading">
                <span
                  class="iconify sidebar-icon icon-[uil--comment-alt-message]"
                  aria-hidden="true"
                ></span>
              </span>
            {:else if item.kind === 'destination' && item.icon}
              <span class="command-palette-leading"
                ><span class="iconify sidebar-icon {item.icon}" aria-hidden="true"></span></span
              >
            {:else if item.kind === 'user' && item.participants?.[0]}
              <span class="command-palette-leading">{@render avatar(item.participants[0])}</span>
            {:else if item.kind === 'dm' && item.participants}
              <span class="command-palette-leading">
                <span class="flex -space-x-2">
                  {#each item.participants.slice(0, 2) as participant (participant.id)}
                    {@render avatar(participant)}
                  {/each}
                </span>
              </span>
            {:else if item.serverLogo}
              {@const logo = item.serverLogo}
              <span
                class="inline-flex h-5 w-5 shrink-0 items-center justify-center overflow-hidden rounded text-[10px] font-bold"
                style:background={logo.logoUrl ? undefined : getGradientForName(logo.name)}
              >
                {#if logo.logoUrl}
                  <img
                    src={logo.logoUrl}
                    alt={logo.name}
                    class="h-full w-full object-cover"
                    onload={(event) =>
                      ((event.currentTarget as HTMLImageElement).style.display = '')}
                    onerror={(event) =>
                      ((event.currentTarget as HTMLImageElement).style.display = 'none')}
                  />
                {:else}
                  <span class="text-white">{logo.name[0]?.toUpperCase() ?? '?'}</span>
                {/if}
              </span>
            {:else}
              <span class="command-palette-leading">#</span>
            {/if}

            {#if item.kind === 'message'}
              <span class="min-w-0 flex-1">
                <span dir="auto" class="line-clamp-2 leading-snug break-words whitespace-pre-line"
                  >{item.label}</span
                >
                {#if item.detail}
                  <span
                    data-testid="message-search-provenance"
                    dir="auto"
                    class="mt-0.5 block truncate text-muted"
                    ><AccountNameTokens
                      text={item.detail}
                      accounts={item.message?.actor
                        ? [
                            {
                              name: item.message.actor.displayName || item.message.actor.login,
                              identity: item.message.actor
                            }
                          ]
                        : []}
                    /></span
                  >
                {/if}
              </span>
            {:else}
              <span class="min-w-0 flex-1 truncate">
                {#if item.kind === 'room'}<span class="text-muted">#</span
                  >{/if}{#if item.kind === 'dm'}<DirectMessageName
                    participants={item.participants ?? []}
                    currentUserId={item.currentUserId}
                  />{:else}<AccountName
                    name={item.label}
                    identity={item.kind === 'user' ? item.participants?.[0] : undefined}
                  />{/if}{#if item.detail}<span class="text-muted"
                    >&nbsp;· <bdi>{item.detail}</bdi></span
                  >{/if}
              </span>
            {/if}

            {#if item.id === 'next-unread'}
              <kbd class="keycap">Alt ⇧ ↓</kbd>
            {:else if index === model.selectedIndex}
              <span class="shrink-0 text-muted" aria-hidden="true">↵</span>
            {/if}
          </button>
        {/each}
      {/if}
    </nav>
  </MenuSection>
</PaletteDialog>
