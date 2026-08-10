<script module lang="ts">
  export type { MessageComposerApi } from './messageComposerState.svelte';
</script>

<script lang="ts">
  import { createMessageAPI } from '$lib/api-client/messages';
  import { createLinkPreviewAPI } from '$lib/api-client/linkPreviews';
  import { m } from '$lib/i18n/messages';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import ConfirmDialog from '$lib/ui/ConfirmDialog.svelte';
  import { getRoomMembers, getRoomMembersStore, getComposerContext } from '$lib/state/room';
  import { shouldAutoFocus } from '$lib/utils/shouldAutoFocus';
  import { timeFormatSettingsFor } from '$lib/utils/formatTime';
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
    autoFocus = true,
    onReady,
    onTyping,
    onMessageSent,
    onCancelReply,
    onEscape,
    showAlsoSendToChannel = false,
    showCreateThread = false
  }: MessageComposerProps = $props();

  const userSettings = $derived(timeFormatSettingsFor(stores.currentUser.user?.settings));
  const composerContext = getComposerContext();
  const state = new MessageComposerState({
    getRoomId: () => roomId,
    getThreadRootEventId: () => inThread,
    getReplyEventId: () => inReplyTo,
    getCanPost: () => canPost,
    getCanAttach: () => canAttach,
    getCanCreateThread: () => showCreateThread,
    getAutoFocus: () => autoFocus,
    getPlaceholder: () => placeholder,
    getOnReady: () => onReady,
    getCallbacks: () => ({ onTyping, onMessageSent, onCancelReply, onEscape }),
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

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  {@attach state.observeResize}
  class="flex flex-col gap-2 p-2"
  onclick={(event) => {
    if (!(event.target as HTMLElement).closest('button, a, input, label, select, .tiptap')) {
      state.editorApi?.focus();
    }
  }}
>
  <ComposerLinkPreview state={state.linkPreviews} />
  <ComposerAttachmentPreviews
    attachments={state.attachments}
    disabled={state.submission.loading}
    getSubmissionStatus={(file) => state.submission.attachmentStatus(file)}
    onremove={(index) => state.attachments.removeFile(index)}
  />

  {#if canAttach && !state.isEditing}
    <input
      bind:this={state.fileInputElement}
      type="file"
      multiple
      onchange={(event) => state.handleFileSelect(event)}
      class="hidden"
    />
  {/if}

  <div
    data-testid="composer-input-surface"
    data-composer-mode={state.isRichComposer ? 'rich' : 'simple'}
    class="composer-mode-surface @container relative flex flex-col rounded-lg bg-surface px-2.5 py-1.5"
    class:opacity-50={state.inputDisabled}
  >
    {#if state.autocomplete.emoji}
      <EmojiAutocomplete
        bind:this={state.autocomplete.emojiRef}
        query={state.autocomplete.emoji.query}
        onSelect={(emoji) => state.autocomplete.selectEmoji(emoji)}
        onClose={() => state.autocomplete.closeEmoji()}
      />
    {/if}

    {#if state.autocomplete.mention}
      <MentionAutocomplete
        bind:this={state.autocomplete.mentionRef}
        query={state.autocomplete.mention.query}
        members={state.mentionCandidates}
        roles={state.mentionRoles}
        onSelect={(login, viaTab) => state.autocomplete.selectMention(login, viaTab)}
        onClose={() => state.autocomplete.closeMention()}
      />
    {/if}

    <div class="min-h-9 min-w-0 px-0.5 py-0.5" data-testid="composer-editor-row">
      {#await tipTapEditorModule}
        <div class="min-h-8 min-w-0" aria-hidden="true"></div>
      {:then { default: TipTapEditor }}
        <TipTapEditor
          placeholder={state.currentPlaceholder}
          editable={!state.inputDisabled}
          autofocus={autoFocus && shouldAutoFocus()}
          testid={state.testid}
          onUpdate={(text) => state.handleEditorUpdate(text)}
          onKeyDown={(event) => state.handleEditorKeyDown(event)}
          onPaste={(event) => state.handlePaste(event)}
          onNextEnterWillSendChange={(value) => (state.editorNextEnterWillSend = value)}
          onRichStructureChange={(value) => (state.editorHasRichStructure = value)}
          onFormattingStateChange={(formatting) => (state.formattingState = { ...formatting })}
          onReady={(api) => state.handleEditorReady(api)}
        />
      {/await}
    </div>

    <ComposerToolbar
      formattingState={state.formattingState}
      editorApi={state.editorApi}
      inputDisabled={state.inputDisabled}
      {canAttach}
      isEditing={state.isEditing}
      canSubmit={state.canSubmit}
      isRichComposer={state.isRichComposer}
      nextEnterWillSend={state.nextEnterWillSend}
      fileInputElement={state.fileInputElement}
      effectiveTimezone={userSettings.effectiveTimezone}
      showCreateThread={showCreateThread && !state.isEditing && !inThread}
      createThread={state.createThread}
      onToggleCreateThread={() => (state.createThread = !state.createThread)}
      showAlsoSendToChannel={(showAlsoSendToChannel && !state.isEditing) ||
        state.showEditEchoToggle}
      alsoSendToChannel={state.alsoSendToChannel}
      onToggleAlsoSendToChannel={() => (state.alsoSendToChannel = !state.alsoSendToChannel)}
      onsubmit={() => state.submit()}
    />
  </div>

  <ComposerModeIndicators
    {inReplyTo}
    {replyDisplayName}
    {replyExcerpt}
    isEditing={state.isEditing}
    oncancelreply={() => onCancelReply?.()}
    oncanceledit={() => state.cancelEdit()}
  />
</div>

{#if state.submission.pendingRoleMentionConfirmation}
  <ConfirmDialog
    title={m('composer.role_mention_confirm_title')}
    tone="warning"
    actionLabel={m('composer.send_anyway')}
    actionIcon="iconify icon-[uil--telegram-alt]"
    loading={state.submission.roleMentionConfirmationLoading}
    onconfirm={() => state.submission.confirmRoleMentionSend()}
    onclose={() => state.submission.cancelRoleMentionConfirmation()}
  >
    {m('composer.role_mention_confirm_body')}
  </ConfirmDialog>
{/if}
