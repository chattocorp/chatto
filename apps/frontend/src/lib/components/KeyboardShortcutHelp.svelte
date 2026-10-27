<!-- @component
Displays a snapshot of the shortcuts available before help took keyboard focus.
The shared dialog owns Escape dismissal and focus restoration.
-->
<script lang="ts">
  import { MenuSection, PaletteDialog, PaletteSearch } from '$lib/ui';
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
      <PaletteSearch
        class="sticky top-0 z-10"
        bind:value={query}
        label={m('search.action')}
        placeholder={m('common.keyboard_shortcuts.title')}
      >
        <button type="button" class="keycap" title={m('ui.close')} onclick={close}>Esc</button>
      </PaletteSearch>
      {#if matches.length === 0}
        <MenuSection>
          <p class="px-3 py-6 text-center text-muted" role="status">
            {m('quick_switcher.no_results')}
          </p>
        </MenuSection>
      {/if}
      {#each ['app', 'editor'] as group (group)}
        {@const entries = matches.filter((entry) => entry.group === group)}
        {#if entries.length}
          <MenuSection
            ariaLabel={group === 'app'
              ? m('common.keyboard_shortcuts.app')
              : m('settings.app_preferences.composer.title')}
          >
            <h2 class="command-palette-heading">
              {group === 'app'
                ? m('common.keyboard_shortcuts.app')
                : m('settings.app_preferences.composer.title')}
            </h2>
            <dl>
              {#each entries as entry (JSON.stringify(entry))}
                <div class="command-palette-row justify-between">
                  <dt>{entry.label}</dt>
                  <dd class="flex shrink-0 justify-end gap-1">
                    {#each shortcutKeys(entry) as key, index (index)}<kbd class="keycap">{key}</kbd
                      >{/each}
                  </dd>
                </div>
              {/each}
            </dl>
          </MenuSection>
        {/if}
      {/each}
    </div>
  {/snippet}
</PaletteDialog>
