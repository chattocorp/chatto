<script lang="ts">
  import { untrack } from 'svelte';
  import * as m from '$lib/i18n/messages';
  import { quickSwitcher } from '$lib/state/globals.svelte';
  import SkeletonImg from '$lib/ui/SkeletonImg.svelte';
  import { getAvatarInitials } from '$lib/utils/initials';
  import { getGradientForName } from '$lib/utils/gradients';
  import { QuickSwitcherModel, type QuickSwitcherAvatarUser } from './quickSwitcherModel.svelte';

  const model = new QuickSwitcherModel();
  let dialogEl: HTMLDialogElement | undefined;

  function syncQuickSwitcherDialog(node: HTMLDialogElement) {
    dialogEl = node;
    const visible = quickSwitcher.visible;

    untrack(() => {
      if (visible) {
        model.activate();
        if (!node.open) node.showModal();
      } else {
        model.deactivate();
        if (node.open) node.close();
      }
    });
  }

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
      const selected = dialogEl?.querySelector(`[data-index="${model.selectedIndex}"]`);
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

<!-- Outer wrapper replicates ContextMenu.svelte's container exactly -->
<dialog
  {@attach syncQuickSwitcherDialog}
  onclose={() => quickSwitcher.close()}
  onkeydown={(event) => {
    if (event.key === 'Escape') event.stopPropagation();
  }}
  oncancel={(event) => {
    event.preventDefault();
    quickSwitcher.close();
  }}
  onclick={(event) => {
    if (event.target === dialogEl) quickSwitcher.close();
  }}
  class="quick-switcher m-auto mt-[15vh] max-h-none max-w-none overflow-visible border-none bg-transparent p-0 text-inherit backdrop:bg-black/50"
>
  {#if quickSwitcher.visible}
    <div
      class="flex w-140 max-w-[90vw] flex-col gap-1 rounded-lg border border-text/10 bg-surface p-1 text-sm shadow-xl"
    >
      <div class="menu-section">
        <div class="flex items-center gap-2 px-3 py-1.5">
          <span class="sidebar-icon iconify text-muted icon-[uil--search]"></span>
          <input
            {@attach registerInput}
            value={model.query}
            oninput={(event) => model.setQuery(event.currentTarget.value)}
            onkeydown={handleKeydown}
            type="text"
            placeholder={m['quick_switcher.placeholder']()}
            class="flex-1 bg-transparent text-text outline-none placeholder:text-muted"
          />
          {#if model.loading}
            <span class="sidebar-icon iconify animate-spin text-muted icon-[uil--spinner-alt]"></span>
          {/if}
          <kbd class="rounded border border-text/10 px-1.5 py-0.5 text-xs text-muted">Esc</kbd>
        </div>
      </div>

      <div class="max-h-80 overflow-y-auto menu-section">
        <nav class="sidebar-nav">
          {#if model.filtered.length === 0 && !model.loading}
            <p class="px-3 py-6 text-center text-muted">
              {model.query.trim() === '?'
                ? m['quick_switcher.message_search.prompt']()
                : model.query.trim().startsWith('?')
                  ? m['quick_switcher.message_search.no_results']()
                  : m['quick_switcher.no_results']()}
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
                  'sidebar-item text-left',
                  item.kind === 'message' ? 'items-start px-2 py-2' : '',
                  index === model.selectedIndex ? 'bg-surface' : ''
                ]}
                onclick={() => model.select(item)}
                onpointerenter={() => model.selectIndex(index)}
              >
                {#if item.kind === 'message'}
                  <span
                    class="mt-0.5 sidebar-icon iconify shrink-0 text-muted icon-[uil--comment-alt-message]"
                  ></span>
                {:else if item.kind === 'destination' && item.icon}
                  <span class="sidebar-icon iconify text-muted {item.icon}"></span>
                {:else if item.kind === 'user'}
                  {@const user = item.participants?.[0] ?? null}
                  <span class="sidebar-icon">
                    {#if user}
                      {@render avatar(user)}
                    {:else}
                      <span class="sidebar-icon iconify text-muted icon-[uil--user]"></span>
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
                    <span class="line-clamp-2 leading-snug break-words whitespace-pre-line"
                      >{item.label}</span
                    >
                    {#if item.detail}
                      <span
                        data-testid="message-search-provenance"
                        class="mt-0.5 block truncate text-muted">{item.detail}</span
                      >
                    {/if}
                  </span>
                {:else}
                  <span class="min-w-0 flex-1 truncate">
                    {#if item.kind === 'room'}<span class="text-muted">#</span
                      >{/if}{item.label}{#if item.detail}<span class="text-muted"
                        >&nbsp;· {item.detail}</span
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
    </div>
  {/if}
</dialog>
