<script lang="ts">
  import EmojiPicker from '$lib/components/EmojiPicker.svelte';
  import ContextMenu from '$lib/ui/ContextMenu.svelte';
  import { Button, Select, TextInput } from '$lib/ui/form';
  import { FormDialog } from '$lib/ui';
  import { toast } from '$lib/ui/toast';
  import {
    deleteCustomStatus as deleteCustomStatusViaAPI,
    setCustomStatus as setCustomStatusViaAPI,
    type CustomUserStatusAPIConfig
  } from '$lib/api-client/userStatus';
  import type { CustomUserStatus } from '$lib/state/userProfiles.svelte';
  import {
    CUSTOM_STATUS_TEMPLATES,
    customStatusTemplateText,
    defaultTemplateExpiry,
    formatCustomStatusText,
    getCustomStatusTemplate,
    type CustomStatusTemplateId
  } from '$lib/customStatusTemplates';
  import { m } from '$lib/i18n/messages';

  type Mode = CustomStatusTemplateId | 'custom';
  type ExpiryPreset =
    'today' | 'thirty_minutes' | 'one_hour' | 'four_hours' | 'tomorrow' | 'never' | 'custom';

  let {
    status,
    config,
    visible = $bindable(true),
    onChange,
    onClose
  }: {
    status?: CustomUserStatus | null;
    config: CustomUserStatusAPIConfig;
    /** Whether the dialog is open. The owner mounts the editor while it is visible. */
    visible?: boolean;
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
  let statusExpiresAt = $state(initialExpiresAt(localStatus));
  let emojiPickerAnchor = $state<{ top: number; bottom: number; left: number } | null>(null);
  let isSaving = $state(false);
  let isClearing = $state(false);
  let error = $state('');
  // svelte-ignore state_referenced_locally
  let expiryPreset = $state<ExpiryPreset>(initialExpiryPreset(localStatus));

  const isCustom = $derived(selectedMode === 'custom');
  const statusTextInputId = 'settings-custom-status-text';
  const expiresAtInputId = 'settings-custom-status-expires-at';
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
  const hasActiveStatus = $derived(!!localStatus);
  const draftIsEmpty = $derived(!statusText.trim());
  const isModified = $derived(
    activeEmoji !== (localStatus?.emoji ?? '') ||
      activeText !== (localStatus?.text ?? '') ||
      statusExpiresAt !== currentExpiresAt
  );
  const canSave = $derived(isModified && (!draftIsEmpty || hasActiveStatus));
  const expiryOptions = $derived([
    { value: 'today', label: m('settings.profile.status.expiry.today') },
    { value: 'thirty_minutes', label: m('settings.profile.status.expiry.thirty_minutes') },
    { value: 'one_hour', label: m('settings.profile.status.expiry.one_hour') },
    { value: 'four_hours', label: m('settings.profile.status.expiry.four_hours') },
    { value: 'tomorrow', label: m('settings.profile.status.expiry.tomorrow') },
    { value: 'never', label: m('settings.profile.status.expiry.never') },
    { value: 'custom', label: m('settings.profile.status.expiry.custom') }
  ]);

  function initialMode(value: CustomUserStatus | null | undefined): Mode {
    return getCustomStatusTemplate(value)?.id ?? 'custom';
  }

  function initialText(value: CustomUserStatus | null | undefined): string {
    return value ? formatCustomStatusText(value.text) : '';
  }

  function initialExpiryPreset(value: CustomUserStatus | null | undefined): ExpiryPreset {
    if (!value) return 'today';
    return value.expiresAt ? 'custom' : 'never';
  }

  function initialExpiresAt(value: CustomUserStatus | null | undefined): string {
    if (value) return toDatetimeLocalValue(value.expiresAt);
    return toLocalDatetime(endOfToday());
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

  function toLocalDatetime(date: Date): string {
    const offset = date.getTimezoneOffset() * 60_000;
    return new Date(date.getTime() - offset).toISOString().slice(0, 16);
  }

  function endOfToday(): Date {
    const now = new Date();
    return new Date(now.getFullYear(), now.getMonth(), now.getDate(), 23, 59, 0, 0);
  }

  function endOfTomorrow(): Date {
    const now = new Date();
    return new Date(now.getFullYear(), now.getMonth(), now.getDate() + 1, 23, 59, 0, 0);
  }

  function updateExpiryFromPreset() {
    switch (expiryPreset) {
      case 'today':
        statusExpiresAt = toLocalDatetime(endOfToday());
        break;
      case 'thirty_minutes':
        statusExpiresAt = toLocalDatetime(new Date(Date.now() + 30 * 60_000));
        break;
      case 'one_hour':
        statusExpiresAt = toLocalDatetime(new Date(Date.now() + 60 * 60_000));
        break;
      case 'four_hours':
        statusExpiresAt = toLocalDatetime(new Date(Date.now() + 4 * 60 * 60_000));
        break;
      case 'tomorrow':
        statusExpiresAt = toLocalDatetime(endOfTomorrow());
        break;
      case 'never':
        statusExpiresAt = '';
        break;
    }
  }

  function selectTemplateDraft(mode: CustomStatusTemplateId) {
    const template = CUSTOM_STATUS_TEMPLATES.find((item) => item.id === mode);
    if (!template) return;
    selectedMode = mode;
    statusEmoji = template.emoji;
    statusText = template.label();
    error = '';

    const defaultExpiry = defaultTemplateExpiry(mode);
    if (defaultExpiry) {
      expiryPreset = mode === 'out_for_lunch' ? 'one_hour' : 'custom';
      statusExpiresAt = toDatetimeLocalValue(defaultExpiry);
    } else {
      expiryPreset = 'never';
      statusExpiresAt = '';
    }
  }

  function markCustomDraft() {
    selectedMode = 'custom';
    error = '';
  }

  function clearDraftStatus() {
    selectedMode = 'custom';
    statusEmoji = '🌿';
    statusText = '';
    expiryPreset = 'today';
    updateExpiryFromPreset();
    error = '';
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
    if (!text && localStatus) {
      await clearCustomStatus();
      return;
    }
    if (!emoji) {
      error = m('settings.profile.status.emoji_required');
      return;
    }
    if (!text) {
      error = m('settings.profile.status.text_required');
      return;
    }

    isSaving = true;
    error = '';

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
      expiryPreset = initialExpiryPreset(customStatus);
      toast.success(m('settings.profile.status.saved'));
      onClose?.();
    } catch (err) {
      error = err instanceof Error ? err.message : m('settings.profile.status.save_failed');
    } finally {
      isSaving = false;
    }
  }

  async function clearCustomStatus() {
    isClearing = true;
    error = '';

    try {
      const customStatus = await deleteCustomStatusViaAPI(config);
      onChange?.(customStatus);
      localStatus = customStatus;
      selectedMode = 'custom';
      statusEmoji = '🌿';
      statusText = '';
      expiryPreset = 'today';
      statusExpiresAt = toLocalDatetime(endOfToday());
      toast.success(m('settings.profile.status.cleared'));
      onClose?.();
    } catch (err) {
      error = err instanceof Error ? err.message : m('settings.profile.status.clear_failed');
    } finally {
      isClearing = false;
    }
  }
</script>

<!--
@component
The "Set a status" dialog for the current user's custom status. It owns the
draft, suggestions, expiry, and the save and clear requests. `FormDialog`
supplies the responsive presentation, including the bottom sheet on narrow
touch screens.
-->

{#snippet emojiTrigger(emoji: string | null | undefined)}
  <button
    type="button"
    class="field-action"
    title={m('settings.profile.status.emoji.choose')}
    aria-label={m('settings.profile.status.emoji.choose')}
    disabled={isSaving || isClearing}
    onclick={openEmojiPicker}
    data-testid="settings-custom-status-emoji-picker"
  >
    <span aria-hidden="true">{emoji || '🙂'}</span>
  </button>
{/snippet}

<FormDialog
  bind:visible
  title={m('settings.profile.status.dialog_title')}
  submitLabel={m('settings.profile.status.save_button')}
  loading={isSaving}
  disabled={!canSave || isClearing}
  error={error || null}
  onsubmit={saveCustomStatus}
  onclose={() => onClose?.()}
>
  <div class="flex flex-col gap-4" data-testid="custom-status-editor">
    <TextInput
      id={statusTextInputId}
      bind:value={statusText}
      label={m('settings.profile.status.text.label')}
      labelHidden
      placeholder={m('settings.profile.status.text.placeholder')}
      disabled={isSaving || isClearing}
      maxlength={100}
      testid="settings-custom-status-text"
      oninput={markCustomDraft}
    >
      {#snippet leading()}
        {@render emojiTrigger(activeEmoji)}
      {/snippet}
      {#snippet trailing()}
        {#if statusText || hasActiveStatus}
          <button
            type="button"
            class="field-action"
            title={m('settings.profile.status.clear_button')}
            aria-label={m('settings.profile.status.clear_button')}
            disabled={isSaving || isClearing}
            onclick={clearDraftStatus}
          >
            <span class="iconify icon-[uil--times]" aria-hidden="true"></span>
          </button>
        {/if}
      {/snippet}
    </TextInput>

    <section class="flex flex-col gap-1" aria-labelledby="custom-status-suggestions">
      <h3 id="custom-status-suggestions" class="text-sm font-semibold text-muted">
        {m('settings.profile.status.suggestions')}
      </h3>
      <div class="-mx-1 selectable-list">
        {#each CUSTOM_STATUS_TEMPLATES as template (template.id)}
          <button
            type="button"
            class="flex w-full cursor-pointer items-center gap-3 selectable-list-item px-2 py-1.5 text-start disabled:cursor-not-allowed disabled:opacity-60"
            disabled={isSaving || isClearing}
            onclick={() => selectTemplateDraft(template.id)}
          >
            <span class="grid w-5 shrink-0 place-items-center" aria-hidden="true">
              {template.emoji}
            </span>
            <span class="min-w-0 truncate font-medium">{template.label()}</span>
          </button>
        {/each}
      </div>
    </section>

    <Select
      id={expiresAtInputId}
      label={m('settings.profile.status.expires_at.label')}
      value={expiryPreset}
      options={expiryOptions}
      disabled={isSaving || isClearing}
      testid="settings-custom-status-expiry-preset"
      onValueChange={(preset) => {
        expiryPreset = preset as ExpiryPreset;
        updateExpiryFromPreset();
      }}
    />

    {#if expiryPreset === 'custom'}
      <TextInput
        id={`${expiresAtInputId}-custom`}
        type="datetime-local"
        label={m('settings.profile.status.expiry.custom_date')}
        bind:value={statusExpiresAt}
        disabled={isSaving || isClearing}
        testid="settings-custom-status-expires-at"
      />
    {/if}
  </div>

  {#snippet secondaryActions()}
    {#if hasActiveStatus}
      <Button
        type="button"
        variant="secondary"
        loading={isClearing}
        disabled={isSaving}
        onclick={clearCustomStatus}
      >
        <span aria-hidden="true" class="iconify icon-[uil--times]"></span>
        {m('settings.profile.status.clear_button')}
      </Button>
    {/if}
  {/snippet}

  {#snippet overlays()}
    {#if emojiPickerAnchor}
      <ContextMenu anchor={emojiPickerAnchor} onclose={() => (emojiPickerAnchor = null)}>
        <EmojiPicker
          serverId={config.serverId}
          onSelect={handleEmojiSelect}
          onClose={() => (emojiPickerAnchor = null)}
        />
      </ContextMenu>
    {/if}
  {/snippet}
</FormDialog>
