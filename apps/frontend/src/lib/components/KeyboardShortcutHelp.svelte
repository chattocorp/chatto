<!-- @component
Displays a snapshot of the shortcuts available before help took keyboard focus.
The shared dialog owns Escape dismissal and focus restoration.
-->
<script lang="ts">
  import { PaletteDialog } from '$lib/ui';
  import { m } from '$lib/i18n/messages';
  import { shortcutKeys, type ShortcutDescription } from '$lib/keyboardShortcuts';

  let {
    visible = $bindable(false),
    shortcuts
  }: {
    visible?: boolean;
    shortcuts: ShortcutDescription[];
  } = $props();
  let query = $state('');
  const matches = $derived(
    shortcuts.filter((entry) =>
      `${entry.label} ${shortcutKeys(entry).join(' ')}`
        .toLocaleLowerCase()
        .includes(query.trim().toLocaleLowerCase())
    )
  );
</script>

<PaletteDialog
  bind:visible
  title={m('common.keyboard_shortcuts.title')}
  onclose={() => (query = '')}
>
  {#snippet children(close)}
    <!-- Keep search in the scroll container so keyboard users can scroll the list.
         The sticky header retains the palette's search placement. -->
    <div
      class="dialog-body flex max-h-[min(24rem,calc(55dvh+4rem))] flex-col gap-1 overflow-y-auto"
    >
      <div class="sticky top-0 z-10 menu-section">
        <div class="flex min-h-10 items-center gap-2 px-3 py-1.5">
          <span class="iconify sidebar-icon icon-[uil--search] text-muted" aria-hidden="true"
          ></span>
          <input
            bind:value={query}
            aria-label={m('search.action')}
            placeholder={m('common.keyboard_shortcuts.title')}
            type="text"
            class="min-w-0 flex-1 bg-transparent text-[16px] text-text outline-none placeholder:text-muted"
          />
          <button type="button" class="keycap" title={m('ui.close')} onclick={close}>Esc</button>
        </div>
      </div>
      <div class="menu-section">
        {#if matches.length === 0}
          <p class="px-3 py-6 text-center text-muted" role="status">
            {m('quick_switcher.no_results')}
          </p>
        {/if}
        {#each ['app', 'editor'] as group (group)}
          {@const entries = matches.filter((entry) => entry.group === group)}
          {#if entries.length}
            <section>
              <h2 class="px-3 pt-2 pb-0.5 text-xs font-medium text-muted uppercase">
                {group === 'app'
                  ? m('common.keyboard_shortcuts.app')
                  : m('settings.app_preferences.composer.title')}
              </h2>
              {#if group === 'editor'}
                <p class="px-3 py-1 text-xs text-muted">
                  {m('common.keyboard_shortcuts.editor_note')}
                </p>
              {/if}
              <dl>
                {#each entries as entry (JSON.stringify(entry))}
                  <div
                    class="flex min-h-8 items-center justify-between gap-2 px-2 py-1 touch-input:min-h-11 touch-input:px-3 touch-input:py-2.5"
                  >
                    <dt>{entry.label}</dt>
                    <dd class="flex shrink-0 justify-end gap-1">
                      {#each shortcutKeys(entry) as key, index (index)}<kbd class="keycap"
                          >{key}</kbd
                        >{/each}
                    </dd>
                  </div>
                {/each}
              </dl>
            </section>
          {/if}
        {/each}
      </div>
    </div>
  {/snippet}
</PaletteDialog>
