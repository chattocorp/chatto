<script lang="ts">
  import EmojiPicker from '$lib/components/EmojiPicker.svelte';
  import ContextMenu from '$lib/ui/ContextMenu.svelte';
  import { Button, FormField } from '$lib/ui/form';
  import { Hint } from '$lib/ui';
  import { toast } from '$lib/ui/toast';
  import {
    deleteCustomStatus as deleteCustomStatusViaAPI,
    updateCustomStatus as updateCustomStatusViaAPI,
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
  import * as m from '$lib/i18n/messages';

  type Mode = CustomStatusTemplateId | 'custom';
  type ExpiryPreset =
    | 'today'
    | 'thirty_minutes'
    | 'one_hour'
    | 'four_hours'
    | 'tomorrow'
    | 'never'
    | 'custom';

  let {
    status,
    config,
    compact = false,
    sheet = false,
    onChange,
    onClose
  }: {
    status?: CustomUserStatus | null;
    config: CustomUserStatusAPIConfig;
    compact?: boolean;
    sheet?: boolean;
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
  let compactCustomEditorOpen = $state(false);
  // svelte-ignore state_referenced_locally
  let expiryPreset = $state<ExpiryPreset>(initialExpiryPreset(localStatus));

  const isCustom = $derived(selectedMode === 'custom');
  const statusTextInputId = $derived(
    compact ? 'compact-custom-status-text' : 'settings-custom-status-text'
  );
  const expiresAtInputId = $derived(
    compact ? 'compact-custom-status-expires-at' : 'settings-custom-status-expires-at'
  );
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
  const hasActiveCustomStatus = $derived(
    !!localStatus && getCustomStatusTemplate(localStatus) === undefined
  );
  const hasActiveStatus = $derived(!!localStatus);
  const customRowActive = $derived(
    selectedMode === 'custom' && (compactCustomEditorOpen || hasActiveCustomStatus)
  );
  const noStatusSelected = $derived(!hasActiveStatus && !compactCustomEditorOpen);
  const draftIsEmpty = $derived(!statusText.trim());
  const isModified = $derived(
    activeEmoji !== (localStatus?.emoji ?? '') ||
      activeText !== (localStatus?.text ?? '') ||
      statusExpiresAt !== currentExpiresAt
  );
  const canSave = $derived(isModified && (!draftIsEmpty || hasActiveStatus));
  const expiryOptions = $derived([
    { value: 'today', label: m['settings.profile.status.expiry.today']() },
    { value: 'thirty_minutes', label: m['settings.profile.status.expiry.thirty_minutes']() },
    { value: 'one_hour', label: m['settings.profile.status.expiry.one_hour']() },
    { value: 'four_hours', label: m['settings.profile.status.expiry.four_hours']() },
    { value: 'tomorrow', label: m['settings.profile.status.expiry.tomorrow']() },
    { value: 'never', label: m['settings.profile.status.expiry.never']() },
    { value: 'custom', label: m['settings.profile.status.expiry.custom']() }
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

  function selectMode(mode: Mode) {
    selectedMode = mode;
    error = '';
    if (mode !== 'custom') {
      compactCustomEditorOpen = false;
      const templateExpiry = defaultTemplateExpiry(mode);
      statusExpiresAt = templateExpiry ? toDatetimeLocalValue(templateExpiry) : '';
    }
  }

  function openCompactCustomEditor() {
    selectMode('custom');
    compactCustomEditorOpen = true;
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
      error = m['settings.profile.status.emoji_required']();
      return;
    }
    if (!text) {
      error = m['settings.profile.status.text_required']();
      return;
    }

    isSaving = true;
    error = '';

    try {
      const customStatus = await updateCustomStatusViaAPI(config, {
        emoji,
        text,
        expiresAt: compact ? null : expiryInputToISO(statusExpiresAt)
      });
      onChange?.(customStatus);
      localStatus = customStatus;
      selectedMode = initialMode(customStatus);
      statusEmoji = customStatus?.emoji ?? statusEmoji;
      statusText = initialText(customStatus);
      statusExpiresAt = toDatetimeLocalValue(customStatus?.expiresAt);
      expiryPreset = initialExpiryPreset(customStatus);
      compactCustomEditorOpen = false;
      toast.success(m['settings.profile.status.saved']());
      onClose?.();
    } catch (err) {
      error = err instanceof Error ? err.message : m['settings.profile.status.save_failed']();
    } finally {
      isSaving = false;
    }
  }

  async function applyTemplateStatus(mode: CustomStatusTemplateId) {
    const template = CUSTOM_STATUS_TEMPLATES.find((item) => item.id === mode);
    if (!template) return;

    isSaving = true;
    error = '';

    try {
      const customStatus = await updateCustomStatusViaAPI(config, {
        emoji: template.emoji,
        text: customStatusTemplateText(mode),
        expiresAt: defaultTemplateExpiry(mode)?.toISOString() ?? null
      });
      onChange?.(customStatus);
      localStatus = customStatus;
      selectedMode = initialMode(customStatus);
      statusEmoji = customStatus?.emoji ?? statusEmoji;
      statusText = initialText(customStatus);
      statusExpiresAt = toDatetimeLocalValue(customStatus?.expiresAt);
      expiryPreset = initialExpiryPreset(customStatus);
      compactCustomEditorOpen = false;
      toast.success(m['settings.profile.status.saved']());
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

    try {
      const customStatus = await deleteCustomStatusViaAPI(config);
      onChange?.(customStatus);
      localStatus = customStatus;
      selectedMode = 'custom';
      statusEmoji = '🌿';
      statusText = '';
      expiryPreset = 'today';
      statusExpiresAt = toLocalDatetime(endOfToday());
      compactCustomEditorOpen = false;
      toast.success(m['settings.profile.status.cleared']());
      onClose?.();
    } catch (err) {
      error = err instanceof Error ? err.message : m['settings.profile.status.clear_failed']();
    } finally {
      isClearing = false;
    }
  }

  async function chooseNoStatus() {
    if (!localStatus) {
      compactCustomEditorOpen = false;
      onClose?.();
      return;
    }

    await clearCustomStatus();
  }
</script>

{#if compact}
  <form
    class="flex flex-col gap-1 menu-section p-1"
    data-testid="custom-status-editor"
    onsubmit={saveCustomStatus}
  >
    <div class="px-2 py-1 text-xs font-semibold text-muted">
      {m['settings.profile.status.title']()}
    </div>
    <div
      class="flex flex-col gap-0.5"
      role="radiogroup"
      aria-label={m['settings.profile.status.template.label']()}
    >
      <button
        type="button"
        role="radio"
        aria-checked={noStatusSelected}
        class={['sidebar-item gap-3 text-left', noStatusSelected && 'bg-surface-100']}
        disabled={isSaving || isClearing}
        onclick={chooseNoStatus}
      >
        <span class="grid w-5 shrink-0 place-items-center" aria-hidden="true">
          <span class="iconify text-muted uil--minus-circle"></span>
        </span>
        <span class={['min-w-0 truncate', noStatusSelected && 'font-medium']}>
          {m['settings.profile.status.template.none']()}
        </span>
        {#if noStatusSelected}
          <span class="ml-auto iconify shrink-0 uil--check" aria-hidden="true"></span>
        {/if}
      </button>
      {#each CUSTOM_STATUS_TEMPLATES as template (template.id)}
        {@const isSelected = selectedMode === template.id}
        <button
          type="button"
          role="radio"
          aria-checked={isSelected}
          class={['sidebar-item gap-3 text-left', isSelected && 'bg-surface-100']}
          disabled={isSaving || isClearing}
          onclick={() => applyTemplateStatus(template.id)}
        >
          <span class="grid w-5 shrink-0 place-items-center" aria-hidden="true">
            {template.emoji}
          </span>
          <span class={['min-w-0 truncate', isSelected && 'font-medium']}>{template.label()}</span>
          {#if isSelected}
            <span class="ml-auto iconify shrink-0 uil--check" aria-hidden="true"></span>
          {/if}
        </button>
      {/each}
      <button
        type="button"
        role="radio"
        aria-checked={hasActiveCustomStatus}
        class={['sidebar-item gap-3 text-left', customRowActive && 'bg-surface-100']}
        disabled={isSaving || isClearing}
        onclick={openCompactCustomEditor}
      >
        {#if hasActiveCustomStatus && localStatus}
          <span class="grid w-5 shrink-0 place-items-center" aria-hidden="true">
            {localStatus.emoji}
          </span>
        {:else}
          <span class="grid w-5 shrink-0 place-items-center" aria-hidden="true">
            <span class="iconify uil--pen"></span>
          </span>
        {/if}
        <span class={['min-w-0 truncate', hasActiveCustomStatus && 'font-medium']}>
          {hasActiveCustomStatus && localStatus
            ? localStatus.text
            : m['settings.profile.status.template.custom']()}
        </span>
        {#if hasActiveCustomStatus}
          <span class="ml-auto iconify shrink-0 uil--check" aria-hidden="true"></span>
        {/if}
      </button>
    </div>

    {#if compactCustomEditorOpen}
      <div class="flex min-w-0 items-center gap-1">
        <button
          type="button"
          class="grid h-8 w-8 shrink-0 cursor-pointer place-items-center rounded-md hover:bg-surface-100 disabled:cursor-not-allowed disabled:opacity-60"
          title={m['settings.profile.status.emoji.choose']()}
          aria-label={m['settings.profile.status.emoji.choose']()}
          disabled={isSaving || isClearing}
          onclick={openEmojiPicker}
          data-testid="settings-custom-status-emoji-picker"
        >
          <span aria-hidden="true">{statusEmoji || '🙂'}</span>
        </button>
        <input
          id={statusTextInputId}
          bind:value={statusText}
          aria-label={m['settings.profile.status.text.label']()}
          placeholder={m['settings.profile.status.text.placeholder']()}
          disabled={isSaving || isClearing}
          maxlength={100}
          class="h-8 input min-w-0 flex-1 rounded-md px-2 py-1 text-sm"
          data-testid="settings-custom-status-text"
        />
        <button
          type="submit"
          class="btn-accent h-8 w-8 shrink-0 !px-0 !py-0 text-sm"
          title={m['settings.profile.status.save_button']()}
          aria-label={m['settings.profile.status.save_button']()}
          disabled={!isModified || isSaving}
        >
          <span
            class={['iconify', isSaving ? 'animate-spin uil--spinner' : 'uil--check']}
            aria-hidden="true"
          ></span>
        </button>
      </div>
    {/if}

    {#if error}
      <Hint tone="danger">{error}</Hint>
    {/if}
  </form>
{:else}
  <form
    class={['flex flex-col', sheet ? 'gap-2' : 'gap-4']}
    data-testid="custom-status-editor"
    onsubmit={saveCustomStatus}
  >
    <div
      class={[
        'flex min-w-0 items-center gap-2 p-2',
        sheet ? 'menu-section' : 'rounded-md border border-border bg-background'
      ]}
    >
      <button
        type="button"
        class="grid h-8 w-8 shrink-0 cursor-pointer place-items-center rounded-md text-lg hover:bg-surface-100 disabled:cursor-not-allowed disabled:opacity-60"
        title={m['settings.profile.status.emoji.choose']()}
        aria-label={m['settings.profile.status.emoji.choose']()}
        disabled={isSaving || isClearing}
        onclick={openEmojiPicker}
        data-testid="settings-custom-status-emoji-picker"
      >
        <span aria-hidden="true">{activeEmoji || '🙂'}</span>
      </button>
      <input
        id={statusTextInputId}
        bind:value={statusText}
        aria-label={m['settings.profile.status.text.label']()}
        placeholder={m['settings.profile.status.text.placeholder']()}
        disabled={isSaving || isClearing}
        maxlength={100}
        class="min-w-0 flex-1 border-0 bg-transparent px-0 py-1 text-base outline-none placeholder:text-muted"
        data-testid="settings-custom-status-text"
        oninput={markCustomDraft}
      />
      {#if statusText || hasActiveStatus}
        <button
          type="button"
          class="grid h-7 w-7 shrink-0 cursor-pointer place-items-center rounded-full text-muted hover:bg-surface-100 hover:text-text disabled:cursor-not-allowed disabled:opacity-60"
          title={m['settings.profile.status.clear_button']()}
          aria-label={m['settings.profile.status.clear_button']()}
          disabled={isSaving || isClearing}
          onclick={clearDraftStatus}
        >
          <span class="iconify uil--times" aria-hidden="true"></span>
        </button>
      {/if}
    </div>

    <div class={sheet ? 'flex flex-col gap-1 menu-section p-1' : 'flex flex-col gap-1.5'}>
      <div
        class={sheet
          ? 'px-2 py-1 text-xs font-semibold text-muted'
          : 'text-sm font-semibold text-muted'}
      >
        {m['settings.profile.status.suggestions']()}
      </div>
      <div class="grid gap-1">
        {#each CUSTOM_STATUS_TEMPLATES as template (template.id)}
          <button
            type="button"
            class="sidebar-item gap-3 text-left"
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
    </div>

    <div class={sheet ? 'menu-section p-2' : ''}>
      <FormField id={expiresAtInputId} label={m['settings.profile.status.expires_at.label']()}>
        <select
          id={expiresAtInputId}
          bind:value={expiryPreset}
          disabled={isSaving || isClearing}
          class="input"
          data-testid="settings-custom-status-expiry-preset"
          onchange={updateExpiryFromPreset}
        >
          {#each expiryOptions as option (option.value)}
            <option value={option.value}>{option.label}</option>
          {/each}
        </select>
      </FormField>
    </div>

    {#if expiryPreset === 'custom'}
      <div class={sheet ? 'menu-section p-2' : ''}>
        <FormField
          id={`${expiresAtInputId}-custom`}
          label={m['settings.profile.status.expiry.custom_date']()}
        >
          <input
            id={`${expiresAtInputId}-custom`}
            type="datetime-local"
            bind:value={statusExpiresAt}
            disabled={isSaving || isClearing}
            class="input"
            data-testid="settings-custom-status-expires-at"
          />
        </FormField>
      </div>
    {/if}

    {#if error}
      <Hint tone="danger">{error}</Hint>
    {/if}

    <div class={['flex flex-wrap items-center justify-end gap-2', sheet && 'menu-section p-2']}>
      {#if hasActiveStatus}
        <Button
          type="button"
          variant="secondary"
          size="sm"
          loading={isClearing}
          disabled={isSaving}
          onclick={clearCustomStatus}
        >
          <span class="iconify uil--times"></span>
          {m['settings.profile.status.clear_button']()}
        </Button>
      {/if}
      <Button type="button" variant="secondary" size="sm" onclick={() => onClose?.()}>
        {m['common.cancel']()}
      </Button>
      <Button
        type="submit"
        size="sm"
        disabled={!canSave || isSaving}
        loading={isSaving || isClearing}
      >
        <span class="iconify uil--check"></span>
        {m['settings.profile.status.save_button']()}
      </Button>
    </div>
  </form>
{/if}

{#if emojiPickerAnchor}
  <ContextMenu anchor={emojiPickerAnchor} onclose={() => (emojiPickerAnchor = null)}>
    <EmojiPicker
      serverId={config.serverId}
      onSelect={handleEmojiSelect}
      onClose={() => (emojiPickerAnchor = null)}
    />
  </ContextMenu>
{/if}
