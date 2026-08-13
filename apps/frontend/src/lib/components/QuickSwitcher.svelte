<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import { quickSwitcher } from '$lib/state/globals.svelte';
  import { Palette } from '$lib/ui';
  import SkeletonImg from '$lib/ui/SkeletonImg.svelte';
  import { getAvatarInitials } from '$lib/utils/initials';
  import { getGradientForName } from '$lib/utils/gradients';
  import { QuickSwitcherModel, type QuickSwitcherAvatarUser } from './quickSwitcherModel.svelte';

  const model = new QuickSwitcherModel();

  // Rebuild from the canonical per-server room stores while the switcher is open.
  $effect(() => model.syncCatalog());
  // Fence transient message plaintext at every server's search privacy boundary.
  $effect(() => model.syncPrivacy());

  function registerInput(node: HTMLInputElement) {
    queueMicrotask(() => node.focus());
  }

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
      const selected = document
        .getElementById('quick-finder-palette')
        ?.querySelector(`[data-index="${model.selectedIndex}"]`);
      selected?.scrollIntoView({ block: 'nearest' });
    });
  }
</script>

{#snippet avatar(user: QuickSwitcherAvatarUser)}
  {#if user.avatarUrl}
    <SkeletonImg
      loading="lazy"
      src={user.avatarUrl}
      alt={user.login}
      class="h-5 w-5 rounded-full object-cover"
    />
  {:else}
    <span
      class="flex h-5 w-5 items-center justify-center rounded-full bg-surface-emphasized text-[10px] font-semibold text-muted"
      aria-label={user.login}
    >
      {getAvatarInitials(user.displayName, user.login)}
    </span>
  {/if}
{/snippet}

<Palette
  id="quick-finder-palette"
  visible={quickSwitcher.visible}
  ariaLabel={m('ui.open_quick_switcher')}
  onopen={() => model.activate()}
  onclosed={() => model.deactivate()}
  onclose={() => quickSwitcher.close()}
>
  <div class="menu-section">
    <div class="flex items-center gap-2 px-3 py-1.5">
      <span class="iconify sidebar-icon icon-[uil--search] text-muted"></span>
      <input
        {@attach registerInput}
        value={model.query}
        oninput={(event) => model.setQuery(event.currentTarget.value)}
        onkeydown={handleKeydown}
        type="text"
        placeholder={m('quick_switcher.placeholder')}
        class="flex-1 bg-transparent text-text outline-none placeholder:text-muted"
      />
      {#if model.loading}
        <span class="iconify sidebar-icon icon-[uil--spinner-alt] animate-spin text-muted"></span>
      {/if}
      <kbd class="rounded border border-text/10 px-1.5 py-0.5 text-xs text-muted">Esc</kbd>
    </div>
  </div>

  <div class="max-h-80 overflow-y-auto menu-section">
    <nav class="sidebar-nav">
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
            <div class="px-3 pt-2 pb-0.5 text-xs font-medium text-muted uppercase">
              {header}
            </div>
          {/if}

          <button
            data-index={index}
            type="button"
            class={[
              'sidebar-item text-start',
              item.kind === 'message' ? 'items-start px-2 py-2' : '',
              index === model.selectedIndex ? 'bg-surface' : ''
            ]}
            onclick={() => model.select(item)}
            onpointerenter={() => model.selectIndex(index)}
          >
            {#if item.kind === 'message'}
              <span
                class="iconify mt-0.5 sidebar-icon icon-[uil--comment-alt-message] shrink-0 text-muted"
              ></span>
            {:else if item.kind === 'destination' && item.icon}
              <span class="iconify sidebar-icon text-muted {item.icon}"></span>
            {:else if item.kind === 'user'}
              {@const user = item.participants?.[0] ?? null}
              <span class="sidebar-icon">
                {#if user}
                  {@render avatar(user)}
                {:else}
                  <span class="iconify sidebar-icon icon-[uil--user] text-muted"></span>
                {/if}
              </span>
            {:else if item.kind === 'dm' && item.participants}
              <span class="sidebar-icon">
                <span class="flex -space-x-2">
                  {#each item.participants as participant (participant.id)}
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
                  <SkeletonImg
                    src={logo.logoUrl}
                    alt={logo.name}
                    class="h-full w-full object-cover"
                  />
                {:else}
                  <span class="text-white">{logo.name[0]?.toUpperCase() ?? '?'}</span>
                {/if}
              </span>
            {:else}
              <span class="sidebar-icon text-muted">#</span>
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
                    class="mt-0.5 block truncate text-muted">{item.detail}</span
                  >
                {/if}
              </span>
            {:else}
              <span class="min-w-0 flex-1 truncate">
                {#if item.kind === 'room'}<span class="text-muted">#</span>{/if}<bdi
                  >{item.label}</bdi
                >{#if item.detail}<span class="text-muted">&nbsp;· <bdi>{item.detail}</bdi></span
                  >{/if}
              </span>
            {/if}

            {#if !model.query.trim()}
              <span class="shrink-0 text-xs text-muted">{model.kindLabels[item.kind]}</span>
            {/if}
          </button>
        {/each}
      {/if}
    </nav>
  </div>
</Palette>
