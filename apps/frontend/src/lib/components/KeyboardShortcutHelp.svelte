<!-- @component
Displays a snapshot of the shortcuts available before help took keyboard focus.
The shared dialog owns Escape dismissal and focus restoration.
-->
<script lang="ts">
  import { Dialog } from '$lib/ui';
  import { Button, TextInput } from '$lib/ui/form';
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
  const searchId = $props.id();
</script>

<Dialog bind:visible title={m('common.keyboard_shortcuts.title')} onclose={() => (query = '')}>
  <div class="flex flex-col gap-6">
    <TextInput
      id={searchId}
      label={m('search.action')}
      bind:value={query}
      leadingIcon="icon-[uil--search]"
    />
    {#if matches.length === 0}<p class="text-muted" role="status">
        {m('quick_switcher.no_results')}
      </p>{/if}
    {#each ['app', 'editor'] as group (group)}
      {@const entries = matches.filter((entry) => entry.group === group)}
      {#if entries.length}
        <section class="flex flex-col gap-3">
          <h2 class="font-semibold">
            {group === 'app'
              ? m('common.keyboard_shortcuts.app')
              : m('settings.app_preferences.composer.title')}
          </h2>
          {#if group === 'editor'}<p class="text-muted">
              {m('common.keyboard_shortcuts.editor_note')}
            </p>{/if}
          <dl class="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-4 gap-y-3">
            {#each entries as entry (JSON.stringify(entry))}
              <dt>{entry.label}</dt>
              <dd class="flex justify-end gap-1">
                {#each shortcutKeys(entry) as key, index (index)}<kbd class="keycap">{key}</kbd
                  >{/each}
              </dd>
            {/each}
          </dl>
        </section>
      {/if}
    {/each}
  </div>
  {#snippet dismissAction()}
    <Button variant="secondary" onclick={() => (visible = false)}>{m('ui.close')}</Button>
  {/snippet}
</Dialog>
