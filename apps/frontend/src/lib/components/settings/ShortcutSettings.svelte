<!-- @component Edit device-local bindings. A focused recording button owns the next chord. -->
<script lang="ts">
  import { usesAppleShortcutModifier } from '$lib/utils/platformShortcuts';
  import { onDestroy } from 'svelte';
  import { m } from '$lib/i18n/messages';
  import {
    CompactActionButton,
    ContextMenu,
    MenuItem,
    MenuSection,
    Panel,
    PageTitle,
    PaneContent,
    PaneHeader
  } from '$lib/ui';
  import { desktopMuteShortcut } from '$lib/desktop/muteShortcut';
  import {
    appShortcutActions,
    bindingText,
    defaultBindings,
    eventChord,
    shortcutLabel,
    shortcutPreferences,
    type ShortcutAction
  } from '$lib/state/shortcutPreferences.svelte';

  let editing = $state<ShortcutAction | null>(null);
  let menu = $state<{ action: ShortcutAction; trigger: HTMLButtonElement; anchor: DOMRect } | null>(
    null
  );
  let revision = 0;

  function closeMenu() {
    menu?.trigger.focus();
    menu = null;
  }

  /** Match the key symbols printed on keyboards and used in shortcut help. */
  function keycaps(chord: string) {
    const symbols: Record<string, string> = {
      Down: '↓',
      Up: '↑',
      Left: '←',
      Right: '→',
      Enter: '↵'
    };
    if (usesAppleShortcutModifier()) symbols.Shift = '⇧';
    return bindingText(chord)
      .split('+')
      .map((key) => symbols[key] ?? key);
  }

  let error = $state('');
  const title = $derived(m('common.keyboard_shortcuts.title'));

  function stop() {
    revision++;
    editing = null;
    error = '';
    shortcutPreferences.recording = false;
  }
  onDestroy(stop);

  async function save(action: ShortcutAction, chord: string) {
    const current = ++revision;
    if (action === 'mute' && chord) {
      const available = await desktopMuteShortcut()
        ?.available(chord)
        .catch(() => false);
      if (revision !== current) return;
      if (!available) {
        error = m('common.keyboard_shortcuts.unavailable');
        return;
      }
    }
    if (!shortcutPreferences.set(action, chord)) {
      error = m('common.keyboard_shortcuts.unavailable');
      return;
    }
    stop();
  }

  function recordingHint(action: ShortcutAction) {
    const modifier = bindingText(action === 'mute' ? 'Control' : 'CommandOrControl');
    const alternative = bindingText(action === 'mute' ? 'Super' : 'Alt');
    return m('common.keyboard_shortcuts.record', { modifier, alternative });
  }

  function record(event: KeyboardEvent, action: ShortcutAction) {
    if (editing !== action || event.key === 'Tab') return;
    event.preventDefault();
    event.stopPropagation();
    if (event.key === 'Escape') {
      stop();
      return;
    }
    if (event.repeat || ['Alt', 'AltGraph', 'Shift', 'Meta', 'Control'].includes(event.key)) return;
    const wrongModifier = usesAppleShortcutModifier() ? event.ctrlKey : event.metaKey;
    const chord =
      action !== 'mute' && wrongModifier ? undefined : eventChord(event, action === 'mute');
    if (chord) save(action, chord);
    else error = recordingHint(action);
  }
</script>

<svelte:window onblur={stop} />

{#snippet row(action: ShortcutAction)}
  {@const label = shortcutLabel(action)}
  {@const chord = shortcutPreferences.bindings[action]}
  {@const menuLabel = m('room.message.actions.more') + ' · ' + label}
  <div class="group flex items-center gap-1 px-1">
    <button
      type="button"
      class={[
        'flex min-h-11 min-w-0 flex-1 cursor-pointer items-center justify-between gap-3 selectable-list-item px-3 py-2 text-start',
        editing === action && 'bg-surface'
      ]}
      aria-label={label + ' · ' + (bindingText(chord) || m('common.edit'))}
      aria-pressed={editing === action}
      onclick={(event) => {
        event.currentTarget.focus();
        editing = action;
        shortcutPreferences.recording = true;
        error = '';
      }}
      onblur={stop}
      onkeydown={(event) => record(event, action)}
    >
      <span class="min-w-0">{label}</span>
      {#if editing === action}
        <span class="max-w-[55%] text-end text-action">{recordingHint(action)}</span>
      {:else if chord}
        <span class="flex shrink-0 gap-1" dir="ltr" aria-hidden="true">
          {#each keycaps(chord) as key, index (index)}<kbd class="keycap min-h-6 min-w-6">{key}</kbd
            >{/each}
        </span>
      {:else}
        <span class="text-muted">{m('common.edit')}</span>
      {/if}
    </button>
    <CompactActionButton
      label={menuLabel}
      touchFriendly
      disabled={!chord && !defaultBindings[action]}
      wrapperClass={menu?.action === action ? '' : 'hover-reveal-action'}
      aria-haspopup="menu"
      aria-expanded={menu?.action === action}
      onclick={(event) => {
        const trigger = event.currentTarget;
        menu = { action, trigger, anchor: trigger.getBoundingClientRect() };
      }}
    >
      <span class="iconify icon-[uil--ellipsis-h]" aria-hidden="true"></span>
    </CompactActionButton>
    {#if menu?.action === action}
      <ContextMenu anchor={menu.anchor} ariaLabel={menuLabel} onclose={closeMenu}>
        <MenuSection>
          {#if chord}
            <MenuItem
              icon="icon-[uil--times]"
              onclick={() => {
                closeMenu();
                void save(action, '');
              }}>{m('common.clear')}</MenuItem
            >
          {/if}
          {#if defaultBindings[action] && chord !== defaultBindings[action]}
            <MenuItem
              icon="icon-[uil--redo]"
              mirrorIconInRtl
              onclick={() => {
                closeMenu();
                void save(action, defaultBindings[action]);
              }}>{m('common.keyboard_shortcuts.reset')}</MenuItem
            >
          {/if}
        </MenuSection>
      </ContextMenu>
    {/if}
  </div>
{/snippet}

<PageTitle {title} scope="app" />
<div class="pane-page">
  <PaneHeader {title} subtitle={m('settings.app_preferences.subtitle')} />
  <PaneContent>
    <div class="flex max-w-3xl flex-col gap-6">
      <Panel noPadding title={m('common.keyboard_shortcuts.app')}>
        <div class="selectable-list">
          {#each appShortcutActions as action (action)}{@render row(action)}{/each}
        </div>
      </Panel>
      {#if desktopMuteShortcut()}
        <Panel noPadding title={m('common.keyboard_shortcuts.system')}
          ><div class="selectable-list">{@render row('mute')}</div></Panel
        >
      {/if}
      {#if error}<p role="alert" class="text-danger">{error}</p>{/if}
    </div>
  </PaneContent>
</div>
