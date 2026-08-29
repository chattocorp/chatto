<script module lang="ts">
  export type { MessageComposerApi } from './messageComposerState.svelte';
</script>

<script lang="ts">
  import { createMessageAPI } from '$lib/api-client/messages';
  import { createLinkPreviewAPI } from '$lib/api-client/linkPreviews';
  import { m } from '$lib/i18n/messages';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import ConfirmDialog from '$lib/ui/ConfirmDialog.svelte';
  import Dialog from '$lib/ui/Dialog.svelte';
  import { Button } from '$lib/ui/form';
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
  import type { Component } from 'svelte';
  import EmojiAutocomplete from './EmojiAutocomplete.svelte';
  import MentionAutocomplete from './MentionAutocomplete.svelte';
  import ComposerLinkPreview from './ComposerLinkPreview.svelte';
  import ComposerAttachmentPreviews from './ComposerAttachmentPreviews.svelte';
  import ComposerFormattingToolbar from './ComposerFormattingToolbar.svelte';
  import ComposerToolbar from './ComposerToolbar.svelte';
  import ComposerModeIndicators from './ComposerModeIndicators.svelte';
  import { MessageComposerState, type MessageComposerProps } from './messageComposerState.svelte';
  import type { ComposerEditorProps } from './editorTypes';
  import { userPreferences, type ComposerEditorKind } from '$lib/state/userPreferences.svelte';

  const editorLoaders: Record<
    ComposerEditorKind,
    () => Promise<{ default: Component<ComposerEditorProps> }>
  > = {
    visual: () => import('./TipTapEditor.svelte'),
    markdown: () => import('./MarkdownEditor.svelte')
  };
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
    onThreadCreated,
    onThreadMessageSent,
    onCancelReply,
    onEscape,
    showAlsoSendToChannel = false,
    showCreateThread = false,
    createThreadRequired = false,
    createThreadDefault = false,
    getRecentThreadRootCandidate = () => null,
    threadsEncouraged = false
  }: MessageComposerProps = $props();

  const clock = new SvelteDate();
  let optimisticPost = $state<{ roomId: string; createdAt: number } | null>(null);
  const slowModeInterval = $derived(formatSlowModeInterval(slowModeSeconds, getLocale()));
  const authoritativeNextPostAt = $derived(
    slowModeNextPostAt ? Date.parse(slowModeNextPostAt) : Number.NaN
  );
  const slowModeDeadline = $derived.by<number | null>(() => {
    if (slowModeSeconds <= 0 || slowModeBypassed) return null;
    const optimisticDeadline =
      optimisticPost?.roomId === roomId
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
  const editorModule = $derived(editorLoaders[userPreferences.composerEditor]());
  const composerId = $props.id();
  const formattingToolbarId = `${composerId}-formatting-toolbar`;

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
    getCreateThreadRequired: () => createThreadRequired,
    getCreateThreadDefault: () => createThreadDefault,
    getRecentThreadRootCandidate: () => getRecentThreadRootCandidate(),
    getAutoFocus: () => autoFocus,
    getComposerSendMode: () => userPreferences.composerSendMode,
    getPlaceholder: () => placeholder,
    getOnReady: () => onReady,
    getCallbacks: () => ({
      onTyping,
      onMessageSent: (event) => {
        clock.setTime(Date.now());
        if (event) optimisticPost = { roomId, createdAt: Date.parse(event.createdAt) };
        onMessageSent?.(event);
      },
      onThreadCreated,
      onThreadMessageSent,
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
      !target.closest('button, a, input, label, select, [data-composer-editor]')
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

  {#if threadsEncouraged && inReplyTo && !inThread}
    <p class="px-0.5 text-xs text-muted" data-testid="threads-encouraged-hint">
      {m('composer.threads_encouraged')}
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

  {#if userPreferences.composerFormattingToolbarVisible}
    <ComposerFormattingToolbar
      id={formattingToolbarId}
      formattingState={composer.formattingState}
      indentState={composer.indentState}
      editorApi={composer.editorApi}
      inputDisabled={composer.inputDisabled}
    />
  {/if}

	<div
		data-testid="composer-input-surface"
		class="composer-surface @container relative flex min-h-12 min-w-0 items-end gap-1 px-2.5 py-1.5"
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

    <button
      type="button"
      onpointerdown={(event) => event.preventDefault()}
      onclick={() =>
        (userPreferences.composerFormattingToolbarVisible =
          !userPreferences.composerFormattingToolbarVisible)}
      aria-label={m('composer.formatting_options')}
      aria-controls={formattingToolbarId}
      aria-expanded={userPreferences.composerFormattingToolbarVisible}
      aria-pressed={userPreferences.composerFormattingToolbarVisible}
      title={m('composer.formatting_options')}
      class={[
        'mb-1.5 flex h-6 w-6 shrink-0 cursor-pointer items-center justify-center rounded text-sm font-semibold transition-[background-color,color,scale] duration-100 active:scale-[0.96]',
        userPreferences.composerFormattingToolbarVisible
          ? 'bg-surface-emphasized text-text'
          : 'text-muted hover:bg-surface-emphasized hover:text-text'
      ]}
    >
      <span aria-hidden="true">Aa</span>
    </button>

    <div
      class="min-h-9 min-w-0 flex-1 px-0.5 py-0.5"
      data-testid="composer-editor-row"
    >
      {#await editorModule}
        <div class="min-h-8 min-w-0" aria-hidden="true"></div>
      {:then { default: Editor }}
        <Editor
          placeholder={composer.currentPlaceholder}
          editable={!composer.inputDisabled}
          autofocus={autoFocus && shouldAutoFocus()}
          testid={composer.testid}
          onUpdate={(text) => composer.handleEditorUpdate(text)}
          onKeyDown={(event) => composer.handleEditorKeyDown(event)}
          onPaste={(event) => composer.handlePaste(event)}
          onFormattingStateChange={(formatting) => (composer.formattingState = { ...formatting })}
          onIndentStateChange={(state) => (composer.indentState = { ...state })}
          onReady={(api) => composer.handleEditorReady(api)}
          onDestroy={(api) => composer.handleEditorDestroyed(api)}
        />
      {/await}
    </div>

    <ComposerToolbar
      editorApi={composer.editorApi}
      inputDisabled={composer.inputDisabled}
      {canAttach}
      isEditing={composer.isEditing}
      canSubmit={composer.canSubmit}
      fileInputElement={composer.fileInputElement}
      effectiveTimezone={userSettings.effectiveTimezone}
      showCreateThread={showCreateThread && !composer.isEditing && !inThread}
      createThread={createThreadRequired || composer.createThread}
      {createThreadRequired}
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

{#if composer.pendingThreadDestinationConfirmation}
  <Dialog
    visible
    size="md"
    title={m('composer.recent_thread_confirm_title')}
    onclose={() => composer.cancelThreadDestinationConfirmation()}
  >
    <p class="text-muted">{m('composer.recent_thread_confirm_body')}</p>

    {#snippet footer()}
      <div class="flex justify-end gap-2">
        <Button variant="secondary" onclick={() => composer.cancelThreadDestinationConfirmation()}>
          {m('common.cancel')}
        </Button>
        <Button variant="secondary" onclick={() => composer.postAsNewRoot()}>
          {m('composer.post_as_new_message')}
        </Button>
        <Button defaultAction variant="action" onclick={() => composer.postInRecentThread()}>
          <span class="iconify icon-[uil--comment-alt-lines]"></span>
          {m('composer.continue_in_thread')}
        </Button>
      </div>
    {/snippet}
  </Dialog>
{/if}
