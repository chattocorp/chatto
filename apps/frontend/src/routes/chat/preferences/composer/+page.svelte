<script lang="ts">
  import { Panel } from '$lib/components/admin';
  import { m } from '$lib/i18n/messages';
  import {
    userPreferences,
    type ComposerEditorKind,
    type ComposerSendMode
  } from '$lib/state/userPreferences.svelte';
  import { ChoiceRow, PageTitle, PaneContent, PaneHeader } from '$lib/ui';

  const editorOptions = $derived([
    {
      value: 'markdown',
      label: m('settings.preferences.editor.markdown.label'),
      description: m('settings.preferences.editor.markdown.description')
    },
    {
      value: 'visual',
      label: m('settings.preferences.editor.visual.label'),
      description: m('settings.preferences.editor.visual.description')
    }
  ] satisfies Array<{ value: ComposerEditorKind; label: string; description: string }>);

  const sendModeOptions = $derived([
    {
      value: 'enter',
      label: m('settings.preferences.send_mode.enter.label'),
      description: m('settings.preferences.send_mode.enter.description')
    },
    {
      value: 'modifier-enter',
      label: m('settings.preferences.send_mode.modifier_enter.label'),
      description: m('settings.preferences.send_mode.modifier_enter.description')
    }
  ] satisfies Array<{ value: ComposerSendMode; label: string; description: string }>);
</script>

<PageTitle title={m('settings.app_preferences.composer.title')} />
<PaneHeader
  title={m('settings.app_preferences.composer.title')}
  subtitle={m('settings.app_preferences.subtitle')}
/>

<PaneContent>
  <div class="flex flex-col gap-6">
    <Panel title={m('settings.preferences.editor.title')} icon="iconify icon-[uil--edit]">
      <div class="max-w-md">
        <p class="mb-3 text-sm text-muted">{m('settings.preferences.browser_scope')}</p>
        <div
          class="flex flex-col gap-2"
          role="radiogroup"
          aria-label={m('settings.preferences.editor.title')}
        >
          {#each editorOptions as option (option.value)}
            <ChoiceRow
              label={option.label}
              description={option.description}
              selected={userPreferences.composerEditor === option.value}
              onclick={() => (userPreferences.composerEditor = option.value)}
            />
          {/each}
        </div>
      </div>
    </Panel>

    <Panel title={m('settings.preferences.send_mode.title')} icon="iconify icon-[uil--message]">
      <div class="max-w-md">
        <p class="mb-3 text-sm text-muted">{m('settings.preferences.browser_scope')}</p>
        <div
          class="flex flex-col gap-2"
          role="radiogroup"
          aria-label={m('settings.preferences.send_mode.title')}
        >
          {#each sendModeOptions as option (option.value)}
            <ChoiceRow
              label={option.label}
              description={option.description}
              selected={userPreferences.composerSendMode === option.value}
              onclick={() => (userPreferences.composerSendMode = option.value)}
            />
          {/each}
        </div>
      </div>
    </Panel>
  </div>
</PaneContent>
