<script lang="ts">
  import EmojiPicker from '$lib/components/EmojiPicker.svelte';
  import ContextMenu from '$lib/ui/ContextMenu.svelte';
  import { Button } from '$lib/ui/form';
  import { Hint } from '$lib/ui';
  import {
    clearCustomStatus as clearCustomStatusViaAPI,
    setCustomStatus as setCustomStatusViaAPI,
    type CustomUserStatusAPIConfig
  } from '$lib/api/userStatus';
  import type { CustomUserStatus } from '$lib/state/userProfiles.svelte';
  import {
    CUSTOM_STATUS_TEMPLATES,
    customStatusTemplateText,
    defaultTemplateExpiry,
    getCustomStatusTemplate,
    type CustomStatusTemplateId
  } from '$lib/customStatusTemplates';
  import * as m from '$lib/i18n/messages';

  type Mode = CustomStatusTemplateId | 'custom';

  let {
    status,
    config,
    compact = false,
    onChange,
    onClose
  }: {
    status?: CustomUserStatus | null;
    config: CustomUserStatusAPIConfig;
    compact?: boolean;
    onChange?: (status: CustomUserStatus | null) => void;
    onClose?: () => void;
  } = $props();

  // Local edit buffer seeded from the current status when the editor mounts.
  // svelte-ignore state_referenced_locally
  let localStatus = $state<CustomUserStatus | null | undefined>(status);
  // svelte-ignore state_referenced_locally
  let selectedMode = $state<Mode>(initialMode(localStatus));
  // svelte-ignore state_referenced_locally
  let statusEmoji = $state(localStatus?.emoji ?? '🌿');
  // svelte-ignore state_referenced_locally
  let statusText = $state(initialText(localStatus));
  // svelte-ignore state_referenced_locally
  let statusExpiresAt = $state(toDatetimeLocalValue(localStatus?.expiresAt));
  let emojiPickerAnchor = $state<{ top: number; bottom: number; left: number } | null>(null);
  let isSaving = $state(false);
  let isClearing = $state(false);
  let error = $state('');
  let successMessage = $state('');

  const isCustom = $derived(selectedMode === 'custom');
  const currentExpiresAt = $derived(toDatetimeLocalValue(localStatus?.expiresAt));
  const activeTemplate = $derived(
    selectedMode === 'custom'
      ? undefined
      : CUSTOM_STATUS_TEMPLATES.find((template) => template.id === selectedMode)
  );
  const activeEmoji = $derived(isCustom ? statusEmoji : (activeTemplate?.emoji ?? statusEmoji));
  const activeText = $derived(
    isCustom ? statusText.trim() : customStatusTemplateText(selectedMode as CustomStatusTemplateId)
  );
  const isModified = $derived(
    activeEmoji !== (localStatus?.emoji ?? '') ||
      activeText !== (localStatus?.text ?? '') ||
      statusExpiresAt !== currentExpiresAt
  );
  const hasActiveStatus = $derived(!!localStatus);

  function initialMode(value: CustomUserStatus | null | undefined): Mode {
    return getCustomStatusTemplate(value)?.id ?? 'custom';
  }

  function initialText(value: CustomUserStatus | null | undefined): string {
    return getCustomStatusTemplate(value) ? '' : (value?.text ?? '');
  }

  function toDatetimeLocalValue(value: string | Date | null | undefined): string {
    if (!value) return '';
    const date = value instanceof Date ? value : new Date(value);
    if (Number.isNaN(date.getTime())) return '';
    const offset = date.getTimezoneOffset() * 60_000;
    return new Date(date.getTime() - offset).toISOString().slice(0, 16);
  }

  function expiryInputToISO(value: string): string | null {
    if (!value) return null;
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? null : date.toISOString();
  }

  function selectMode(mode: Mode) {
    selectedMode = mode;
    error = '';
    successMessage = '';
    if (mode !== 'custom') {
      const templateExpiry = defaultTemplateExpiry(mode);
      statusExpiresAt = templateExpiry ? toDatetimeLocalValue(templateExpiry) : '';
    }
  }

  function openEmojiPicker(event: MouseEvent) {
    const rect = (event.currentTarget as HTMLElement).getBoundingClientRect();
    emojiPickerAnchor = { top: rect.top, bottom: rect.bottom, left: rect.left };
  }

  function handleEmojiSelect(emoji: string) {
    statusEmoji = emoji;
    emojiPickerAnchor = null;
  }

  async function saveCustomStatus(event: Event) {
    event.preventDefault();
    const emoji = activeEmoji.trim();
    const text = activeText.trim();
    if (!emoji) {
      error = m['settings.profile.status.emoji_required']();
      return;
    }
    if (!text) {
      error = m['settings.profile.status.text_required']();
      return;
    }

    isSaving = true;
    error = '';
    successMessage = '';

    try {
      const customStatus = await setCustomStatusViaAPI(config, {
        emoji,
        text,
        expiresAt: expiryInputToISO(statusExpiresAt)
      });
      onChange?.(customStatus);
      localStatus = customStatus;
      selectedMode = initialMode(customStatus);
      statusEmoji = customStatus?.emoji ?? statusEmoji;
      statusText = initialText(customStatus);
      statusExpiresAt = toDatetimeLocalValue(customStatus?.expiresAt);
      successMessage = m['settings.profile.status.saved']();
      onClose?.();
    } catch (err) {
      error = err instanceof Error ? err.message : m['settings.profile.status.save_failed']();
    } finally {
      isSaving = false;
    }
  }

  async function clearCustomStatus() {
    isClearing = true;
    error = '';
    successMessage = '';

    try {
      const customStatus = await clearCustomStatusViaAPI(config);
      onChange?.(customStatus);
      localStatus = customStatus;
      selectedMode = 'custom';
      statusEmoji = '🌿';
      statusText = '';
      statusExpiresAt = '';
      successMessage = m['settings.profile.status.cleared']();
      onClose?.();
    } catch (err) {
      error = err instanceof Error ? err.message : m['settings.profile.status.clear_failed']();
    } finally {
      isClearing = false;
    }
  }
</script>

<form
  class={['flex flex-col gap-4', compact ? 'menu-section w-96 max-w-[calc(100vw-2rem)] p-3' : '']}
  data-testid="custom-status-editor"
  onsubmit={saveCustomStatus}
>
  <div class="flex flex-col gap-2">
    <span class="text-sm font-medium text-text">{m['settings.profile.status.template.label']()}</span>
    <div
      class="flex flex-col gap-2"
      role="radiogroup"
      aria-label={m['settings.profile.status.template.label']()}
    >
      {#each CUSTOM_STATUS_TEMPLATES as template (template.id)}
        {@const isSelected = selectedMode === template.id}
        <button
          type="button"
          role="radio"
          aria-checked={isSelected}
          class={['choice-row', isSelected && 'choice-row-selected']}
          onclick={() => selectMode(template.id)}
        >
          <span class={['choice-indicator', isSelected && 'choice-indicator-selected']}>
            {#if isSelected}
              <span class="choice-indicator-dot"></span>
            {/if}
          </span>
          <span class="flex min-w-0 items-center gap-2">
            <span aria-hidden="true">{template.emoji}</span>
            <span class={['min-w-0 truncate', isSelected && 'font-medium']}>{template.label()}</span>
          </span>
        </button>
      {/each}
      <button
        type="button"
        role="radio"
        aria-checked={selectedMode === 'custom'}
        class={['choice-row', selectedMode === 'custom' && 'choice-row-selected']}
        onclick={() => selectMode('custom')}
      >
        <span class={['choice-indicator', selectedMode === 'custom' && 'choice-indicator-selected']}>
          {#if selectedMode === 'custom'}
            <span class="choice-indicator-dot"></span>
          {/if}
        </span>
        <span class="flex min-w-0 items-center gap-2">
          <span class="iconify uil--edit-alt" aria-hidden="true"></span>
          <span class={['min-w-0 truncate', selectedMode === 'custom' && 'font-medium']}>
            {m['settings.profile.status.template.custom']()}
          </span>
        </span>
      </button>
    </div>
  </div>

  {#if isCustom}
    <label class="flex flex-col gap-1 text-sm">
      <span class="font-medium text-text">{m['settings.profile.status.text.label']()}</span>
      <span class="flex min-w-0 items-center gap-2">
        <button
          type="button"
          class="btn-secondary h-10 w-10 shrink-0 !px-0 text-xl"
          title={m['settings.profile.status.emoji.choose']()}
          aria-label={m['settings.profile.status.emoji.choose']()}
          disabled={isSaving || isClearing}
          onclick={openEmojiPicker}
          data-testid="settings-custom-status-emoji-picker"
        >
          <span aria-hidden="true">{statusEmoji || '🙂'}</span>
        </button>
        <input
          bind:value={statusText}
          placeholder={m['settings.profile.status.text.placeholder']()}
          disabled={isSaving || isClearing}
          maxlength={100}
          class="input min-w-0 flex-1"
          data-testid="settings-custom-status-text"
        />
      </span>
    </label>
  {/if}

  <label class="flex flex-col gap-1 text-sm">
    <span class="font-medium text-text">{m['settings.profile.status.expires_at.label']()}</span>
    <input
      type="datetime-local"
      bind:value={statusExpiresAt}
      disabled={isSaving || isClearing}
      class="input"
      data-testid="settings-custom-status-expires-at"
    />
  </label>

  {#if error}
    <Hint tone="danger">{error}</Hint>
  {/if}
  {#if successMessage && !compact}
    <Hint tone="success">{successMessage}</Hint>
  {/if}

  <div class="flex flex-nowrap items-center justify-end gap-2">
    {#if hasActiveStatus}
      <Button type="button" variant="secondary" size="sm" loading={isClearing} onclick={clearCustomStatus}>
        <span class="iconify uil--times"></span>
        {m['settings.profile.status.clear_button']()}
      </Button>
    {/if}
    <Button type="submit" size="sm" disabled={!isModified || isSaving} loading={isSaving}>
      <span class="iconify uil--check"></span>
      {m['settings.profile.status.save_button']()}
    </Button>
  </div>
</form>

{#if emojiPickerAnchor}
  <ContextMenu anchor={emojiPickerAnchor} onclose={() => (emojiPickerAnchor = null)}>
    <EmojiPicker
      serverId={config.serverId}
      onSelect={handleEmojiSelect}
      onClose={() => (emojiPickerAnchor = null)}
    />
  </ContextMenu>
{/if}
