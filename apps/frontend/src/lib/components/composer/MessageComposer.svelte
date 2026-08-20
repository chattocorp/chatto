<script module lang="ts">
  export type { MessageComposerApi } from './messageComposerState.svelte';
</script>

<script lang="ts">
  import { createMessageAPI } from '$lib/api-client/messages';
  import { createLinkPreviewAPI } from '$lib/api-client/linkPreviews';
  import { m } from '$lib/i18n/messages';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import ConfirmDialog from '$lib/ui/ConfirmDialog.svelte';
  import { toast } from '$lib/ui/toast';
  import { getRoomMembers, getRoomMembersStore, getComposerContext } from '$lib/state/room';
  import { shouldAutoFocus } from '$lib/utils/shouldAutoFocus';
  import { timeFormatSettingsFor } from '$lib/utils/formatTime';
  import { getLocale } from '$lib/i18n/runtime';
  import {
    formatSlowModeCountdown,
    formatSlowModeInterval,
    slowModeRemainingSeconds as remainingSlowModeSeconds
  } from '$lib/slowMode';
  import { Code, ConnectError } from '$lib/api-client/connect';
  import { SvelteDate } from 'svelte/reactivity';
  import EmojiAutocomplete from './EmojiAutocomplete.svelte';
  import MentionAutocomplete from './MentionAutocomplete.svelte';
  import ComposerLinkPreview from './ComposerLinkPreview.svelte';
  import ComposerAttachmentPreviews from './ComposerAttachmentPreviews.svelte';
  import ComposerToolbar from './ComposerToolbar.svelte';
  import ComposerModeIndicators from './ComposerModeIndicators.svelte';
  import { MessageComposerState, type MessageComposerProps } from './messageComposerState.svelte';

  const tipTapEditorModule = import('./TipTapEditor.svelte');
  const serverScope = useServerScope();
  const stores = serverScope.store;
  const serverInfo = stores.serverInfo;
  const roomUnreadStore = stores.roomUnread;
  const mentionRolesStore = stores.mentionRoles;

  let {
    roomId,
    inThread,
    inReplyTo,
    replyDisplayName,
    replyExcerpt,
    placeholder,
    canPost = true,
    canAttach = true,
    slowModeSeconds = 0,
    slowModeNextPostAt = null,
    slowModeBypassed = false,
    autoFocus = true,
    onReady,
    onTyping,
    onMessageSent,
    onCancelReply,
    onEscape,
    showAlsoSendToChannel = false,
    showCreateThread = false
  }: MessageComposerProps = $props();

  const clock = new SvelteDate();
  let optimisticPost = $state<{ roomId: string; createdAt: number } | null>(null);
  const slowModeInterval = $derived(formatSlowModeInterval(slowModeSeconds, getLocale()));
  const authoritativeNextPostAt = $derived(
    slowModeNextPostAt ? Date.parse(slowModeNextPostAt) : Number.NaN
  );
  const slowModeDeadline = $derived.by<number | null>(() => {
    if (slowModeSeconds <= 0 || slowModeBypassed) return null;
    const optimisticDeadline = optimisticPost?.roomId === roomId
      ? optimisticPost.createdAt + slowModeSeconds * 1000
      : Number.NaN;
    const deadlines = [authoritativeNextPostAt, optimisticDeadline].filter(Number.isFinite);
    return deadlines.length > 0 ? Math.max(...deadlines) : null;
  });
  const slowModeRemainingSeconds = $derived(
    Math.min(
      Math.max(0, slowModeSeconds),
      remainingSlowModeSeconds(slowModeDeadline, clock.getTime())
    )
  );
  const slowModeBlocked = $derived(slowModeRemainingSeconds > 0);

  $effect(() => {
    const deadline = slowModeDeadline;
    const now = clock.getTime();
    if (deadline === null || deadline <= now) return;
    const remaining = deadline - now;
    const delay = remaining % 1000 || 1000;
    const timeout = window.setTimeout(() => clock.setTime(Date.now()), delay);
    return () => window.clearTimeout(timeout);
  });

  const userSettings = $derived(timeFormatSettingsFor(stores.currentUser.user?.settings));
  const composerContext = getComposerContext();
  const composer = new MessageComposerState({
    getRoomId: () => roomId,
    getThreadRootEventId: () => inThread,
    getReplyEventId: () => inReplyTo,
    getCanPost: () => canPost,
    getCanAttach: () => canAttach,
    getSlowModeBlocked: () => slowModeBlocked,
    getCanCreateThread: () => showCreateThread,
    getAutoFocus: () => autoFocus,
    getPlaceholder: () => placeholder,
    getOnReady: () => onReady,
    getCallbacks: () => ({
      onTyping,
      onMessageSent: (event) => {
        clock.setTime(Date.now());
        if (event) optimisticPost = { roomId, createdAt: Date.parse(event.createdAt) };
        onMessageSent?.(event);
      },
      onCancelReply,
      onEscape
    }),
    onPostError: (error) => {
      if (slowModeSeconds <= 0 || !(error instanceof ConnectError)) return false;
      if (error.code !== Code.ResourceExhausted) return false;
      toast.error(m('composer.slow_mode_rejected'));
      return true;
    },
    context: composerContext,
    getMembers: getRoomMembers,
    membersStore: getRoomMembersStore(),
    mentionRolesStore,
    serverInfo,
    roomUnreadStore,
    getMessageAPI: () => serverScope.connection.getAPI(createMessageAPI),
    getLinkPreviewAPI: () => serverScope.connection.getAPI(createLinkPreviewAPI),
    isConnectionLost: () => serverScope.connection.showConnectionLostBanner
  });
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  {@attach composer.observeResize}
  class="flex flex-col gap-2 p-2"
  onpointerdown={(event) => {
    const target = event.target;
    // Keep this on pointerdown: a release-time click after selecting into the
    // padding would refocus the editor and collapse the selection.
    if (
      event.button === 0 &&
      target instanceof Element &&
      !target.closest('button, a, input, label, select, .tiptap')
    ) {
      composer.editorApi?.focus();
    }
  }}
>
  <ComposerLinkPreview state={composer.linkPreviews} />
  <ComposerAttachmentPreviews
    attachments={composer.attachments}
    disabled={composer.submission.loading}
    getSubmissionStatus={(file) => composer.submission.attachmentStatus(file)}
    onremove={(index) => composer.attachments.removeFile(index)}
  />

  {#if slowModeSeconds > 0}
    <p class="px-0.5 text-xs text-muted" data-testid="slow-mode-status" aria-live="polite">
      {#if slowModeBypassed}
        {m('composer.slow_mode_bypassed', { interval: slowModeInterval })}
      {:else if slowModeBlocked}
        {m('composer.slow_mode_waiting', {
          countdown: formatSlowModeCountdown(slowModeRemainingSeconds)
        })}
      {:else}
        {m('composer.slow_mode_ready', { interval: slowModeInterval })}
      {/if}
    </p>
  {/if}

  {#if canAttach && !composer.isEditing}
    <input
      bind:this={composer.fileInputElement}
      type="file"
      multiple
      onchange={(event) => composer.handleFileSelect(event)}
      class="hidden"
    />
  {/if}

  <div
    data-testid="composer-input-surface"
    data-composer-mode={composer.isRichComposer ? 'rich' : 'simple'}
    class="composer-mode-surface @container relative flex flex-col rounded-lg bg-surface px-2.5 py-1.5"
    class:opacity-50={composer.inputDisabled}
  >
    {#if composer.autocomplete.emoji}
      <EmojiAutocomplete
        bind:this={composer.autocomplete.emojiRef}
        query={composer.autocomplete.emoji.query}
        onSelect={(emoji) => composer.autocomplete.selectEmoji(emoji)}
        onClose={() => composer.autocomplete.closeEmoji()}
      />
    {/if}

    {#if composer.autocomplete.mention}
      <MentionAutocomplete
        bind:this={composer.autocomplete.mentionRef}
        query={composer.autocomplete.mention.query}
        members={composer.mentionCandidates}
        roles={composer.mentionRoles}
        onSelect={(login, viaTab) => composer.autocomplete.selectMention(login, viaTab)}
        onClose={() => composer.autocomplete.closeMention()}
      />
    {/if}

    <div class="min-h-9 min-w-0 px-0.5 py-0.5" data-testid="composer-editor-row">
      {#await tipTapEditorModule}
        <div class="min-h-8 min-w-0" aria-hidden="true"></div>
      {:then { default: TipTapEditor }}
        <TipTapEditor
          placeholder={composer.currentPlaceholder}
          editable={!composer.inputDisabled}
          autofocus={autoFocus && shouldAutoFocus()}
          testid={composer.testid}
          onUpdate={(text) => composer.handleEditorUpdate(text)}
          onKeyDown={(event) => composer.handleEditorKeyDown(event)}
          onPaste={(event) => composer.handlePaste(event)}
          onNextEnterWillSendChange={(value) => (composer.editorNextEnterWillSend = value)}
          onRichStructureChange={(value) => (composer.editorHasRichStructure = value)}
          onFormattingStateChange={(formatting) => (composer.formattingState = { ...formatting })}
          onReady={(api) => composer.handleEditorReady(api)}
        />
      {/await}
    </div>

    <ComposerToolbar
      formattingState={composer.formattingState}
      editorApi={composer.editorApi}
      inputDisabled={composer.inputDisabled}
      {canAttach}
      isEditing={composer.isEditing}
      canSubmit={composer.canSubmit}
      isRichComposer={composer.isRichComposer}
      nextEnterWillSend={composer.nextEnterWillSend}
      fileInputElement={composer.fileInputElement}
      effectiveTimezone={userSettings.effectiveTimezone}
      showCreateThread={showCreateThread && !composer.isEditing && !inThread}
      createThread={composer.createThread}
      onToggleCreateThread={() => (composer.createThread = !composer.createThread)}
      showAlsoSendToChannel={(showAlsoSendToChannel && !composer.isEditing) ||
        composer.showEditEchoToggle}
      alsoSendToChannel={composer.alsoSendToChannel}
      onToggleAlsoSendToChannel={() => (composer.alsoSendToChannel = !composer.alsoSendToChannel)}
      onsubmit={() => composer.submit()}
    />
  </div>

  <ComposerModeIndicators
    {inReplyTo}
    {replyDisplayName}
    {replyExcerpt}
    isEditing={composer.isEditing}
    oncancelreply={() => onCancelReply?.()}
    oncanceledit={() => composer.cancelEdit()}
  />
</div>

{#if composer.submission.pendingRoleMentionConfirmation}
  <ConfirmDialog
    title={m('composer.role_mention_confirm_title')}
    tone="warning"
    actionLabel={m('composer.send_anyway')}
    actionIcon="iconify icon-[uil--telegram-alt]"
    loading={composer.submission.roleMentionConfirmationLoading}
    onconfirm={() => composer.submission.confirmRoleMentionSend()}
    onclose={() => composer.submission.cancelRoleMentionConfirmation()}
  >
    {m('composer.role_mention_confirm_body')}
  </ConfirmDialog>
{/if}
