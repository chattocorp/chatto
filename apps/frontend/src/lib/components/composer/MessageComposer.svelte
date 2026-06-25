<script lang="ts">
  import { onDestroy, tick, untrack } from 'svelte';
  import { graphql, useFragment } from '$lib/gql';
  import { RoomEventViewFragmentDoc, type RoomEventViewFragment } from '$lib/gql/graphql';
  import { createMessageAPI } from '$lib/api/messages';
  import * as m from '$lib/i18n/messages';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { parseMessageLink } from '$lib/messageLinks';
  import LinkPreviewCard from '$lib/components/LinkPreviewCard.svelte';
  import LinkPreviewSkeleton from '$lib/components/LinkPreviewSkeleton.svelte';
  import MessagePreviewCard from '$lib/components/MessagePreviewCard.svelte';
  import ConfirmDialog from '$lib/ui/ConfirmDialog.svelte';
  import { toast } from '$lib/ui/toast';
  import {
    getRoomMembers,
    getRoomMembersStore,
    getComposerContext,
    type QuoteInsertionContent,
    type RoomMember
  } from '$lib/state/room';
  import { shouldAutoFocus } from '$lib/utils/shouldAutoFocus';
  import { isTouchDevice } from '$lib/utils/isTouchDevice';
  import { hasVisibleContent } from '$lib/validation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import EmojiAutocomplete from '$lib/components/composer/EmojiAutocomplete.svelte';
  import MentionAutocomplete from '$lib/components/composer/MentionAutocomplete.svelte';
  import type { TipTapEditorApi } from './TipTapEditor.svelte';
  import { DraftState, draftKey } from './draft.svelte';
  import { AttachmentsState } from './attachments.svelte';
  import { LinkPreviewState } from './linkPreviews.svelte';
  import { AutocompleteState, type MentionRole } from './autocomplete.svelte';

  const tipTapEditorModule = import('./TipTapEditor.svelte');

  type ShortcutHints = {
    submit: string;
    enterAgain: string;
  };

  function getShortcutHints(): ShortcutHints | null {
    if (typeof navigator === 'undefined' || isTouchDevice()) return null;

    const userAgentDataPlatform =
      'userAgentData' in navigator
        ? (navigator.userAgentData as { platform?: string } | undefined)?.platform
        : undefined;
    const platform = userAgentDataPlatform ?? navigator.platform ?? '';
    const usesReturn = /Mac|iPhone|iPad|iPod/i.test(platform);
    return usesReturn
      ? { submit: 'Cmd+Return to Send', enterAgain: 'Return again to Send' }
      : { submit: 'Ctrl+Return to Send', enterAgain: 'Enter again to Send' };
  }

  const stores = serverRegistry.getStore(getActiveServer());
  const serverInfo = stores.serverInfo;
  const roomUnreadStore = stores.roomUnread;

  export type MessageComposerApi = {
    addFiles: (files: File[]) => void;
    focus: () => void;
    insertQuote: (text: QuoteInsertionContent) => void;
  };

  let {
    roomId,
    inThread,
    inReplyTo,
    replyDisplayName,
    replyExcerpt,
    placeholder: customPlaceholder,
    canPost = true,
    canAttach = true,
    autoFocus = true,
    onReady,
    onTyping,
    onMessageSent,
    onCancelReply,
    onEscape,
    showAlsoSendToChannel = false
  }: {
    roomId: string;
    inThread?: string;
    inReplyTo?: string;
    replyDisplayName?: string;
    replyExcerpt?: string;
    placeholder?: string;
    canPost?: boolean;
    canAttach?: boolean;
    autoFocus?: boolean;
    onReady?: (api: MessageComposerApi) => void;
    onTyping?: () => void;
    onMessageSent?: (event: RoomEventViewFragment | null) => void;
    onCancelReply?: () => void;
    onEscape?: () => void;
    showAlsoSendToChannel?: boolean;
  } = $props();

  const connection = useConnection();

  let alsoSendToChannel = $state(false);

  // Get room members from context (provided by Room.svelte)
  const members = $derived(getRoomMembers());
  const membersStore = getRoomMembersStore();
  let mentionSearchMembers = $state.raw<RoomMember[]>([]);
  let mentionSearchTimer: ReturnType<typeof setTimeout> | null = null;
  let mentionSearchRequestId = 0;
  const mentionCandidateMembers = $derived(
    mentionSearchMembers.length > 0 ? mentionSearchMembers : members
  );

  onDestroy(() => {
    if (mentionSearchTimer) clearTimeout(mentionSearchTimer);
  });

  const composerContext = getComposerContext();
  const editState = composerContext.editState;
  const quoteInsertionState = composerContext.quoteInsertionState;
  const lastEditableMessageCtx = composerContext.lastEditableMessage;
  const scrollState = composerContext.scrollState;
  const isEditing = $derived(editState.eventId !== null);
  const showEditEchoCheckbox = $derived(
    isEditing &&
      editState.threadRootEventId !== null &&
      (editState.channelEchoEventId !== null || editState.canAddChannelEcho)
  );

  // When the composer resizes (editor grows/shrinks, attachments added/removed),
  // scroll to bottom if sticky. This replaces the synchronous scrollToBottomIfSticky()
  // that was lost when the old textarea's autoResize() was removed during TipTap migration.
  function observeComposerResize(node: HTMLDivElement) {
    if (!scrollState) return;
    const observer = new ResizeObserver(() => {
      scrollState.scrollToBottomIfSticky();
    });
    observer.observe(node);
    return () => observer.disconnect();
  }

  const DRAFT_KEY = $derived(draftKey(roomId, inThread));
  let message = $state('');

  // TipTap editor API (received via onReady callback)
  let editorApi = $state<TipTapEditorApi | null>(null);
  const draftState = new DraftState();
  const attachments = new AttachmentsState(() => serverInfo);
  const linkPreviews = new LinkPreviewState(() => connection().client);
  const autocomplete = new AutocompleteState(
    () => editorApi,
    () => mentionCandidateMembers,
    () => mentionRoles
  );
  let mentionRoles = $state<MentionRole[]>([]);

  $effect(() => {
    const query = autocomplete.mention?.query ?? null;
    const requestId = ++mentionSearchRequestId;

    if (mentionSearchTimer) {
      clearTimeout(mentionSearchTimer);
      mentionSearchTimer = null;
    }

    if (!query) {
      mentionSearchMembers = [];
      return;
    }

    mentionSearchTimer = setTimeout(() => {
      void membersStore.searchMembers(query).then((results) => {
        if (requestId !== mentionSearchRequestId) return;
        mentionSearchMembers = results;
      });
    }, 150);
  });

  // Dynamic placeholder changes between normal and edit mode
  let currentPlaceholder = $derived(
    isEditing
      ? m['composer.editing_placeholder']()
      : (customPlaceholder ?? m['composer.placeholder']())
  );

  // Testid for E2E tests - distinguishes main input from thread reply input
  let testid = $derived(inThread ? 'thread-reply-input' : 'message-input');
  const shortcutHints = getShortcutHints();

  // Track editing transitions by event identity so editor setContent() doesn't
  // run repeatedly while TipTap echoes updates back through onUpdate.
  let editSeededForEvent = '';

  // When entering edit mode, pre-fill with original message body and clear any pending attachments.
  // When exiting edit mode (cancelled or message deleted), clear the input.
  $effect(() => {
    const eventId = editState.eventId;
    const originalBody = editState.originalBody;
    const api = editorApi;

    if (eventId && originalBody && editSeededForEvent !== eventId) {
      editSeededForEvent = eventId;
      autocomplete.reset();
      draftState.clearText();
      message = originalBody;
      manualRichMode = false;
      alsoSendToChannel = editState.channelEchoEventId !== null;
      api?.setContent(originalBody);
      tick().then(() => api?.focus('end'));
      attachments.clear();
      linkPreviews.clear();
    } else if (editSeededForEvent && !eventId) {
      // Exiting edit mode - clear the input
      autocomplete.reset();
      message = '';
      manualRichMode = false;
      alsoSendToChannel = false;
      editSeededForEvent = '';
      api?.setContent('');
    }
  });

  // Load draft from sessionStorage when room changes
  // Using sessionStorage (not localStorage) so drafts are tab-specific
  let autocompleteResetRoomId = '';
  $effect(() => {
    if (autocompleteResetRoomId !== roomId) {
      autocompleteResetRoomId = roomId;
      autocomplete.resetForRoom();
    }

    if (isEditing) {
      draftState.switchKey(DRAFT_KEY);
      attachments.restore([]);
      return;
    }

    const draft = draftState.switchKey(DRAFT_KEY);
    message = draft;
    manualRichMode = false;
    editorApi?.setContent(draft);
    attachments.restore(untrack(() => draftState.takeFiles()));

    return () => {
      draftState.stashFiles(untrack(() => attachments.filesWithUrls));
    };
  });

  // Persist draft to sessionStorage
  $effect(() => {
    void DRAFT_KEY;
    if (isEditing) return;
    draftState.persistText(message);
  });

  const PostMessageMutation = graphql(`
    mutation PostMessage($input: PostMessageInput!) {
      postMessage(input: $input) {
        ...RoomEventView
      }
    }
  `);

  const ComposerMentionRolesQuery = graphql(`
    query ComposerMentionRoles {
      server {
        roles {
          name
          isSystem
          position
          pingable
        }
      }
    }
  `);

  const UpdateMessageMutation = graphql(`
    mutation UpdateMessageFromInput($input: UpdateMessageInput!) {
      updateMessage(input: $input)
    }
  `);

  $effect(() => {
    return linkPreviews.scheduleDetection(message, isEditing);
  });

  $effect(() => {
    const client = connection().client;
    let cancelled = false;

    async function loadMentionRoles() {
      const response = await client.query(ComposerMentionRolesQuery, {});
      if (cancelled) return;
      if (response.error) {
        mentionRoles = [];
        return;
      }
      mentionRoles =
        response.data?.server?.roles
          .filter((role) => role.name !== 'everyone')
          .map((role) => ({
            name: role.name,
            isSystem: role.isSystem,
            position: role.position,
            pingable: role.pingable
          })) ?? [];
    }

    void loadMentionRoles();
    return () => {
      cancelled = true;
    };
  });

  let loading = $state(false);
  let fileInputElement = $state<HTMLInputElement>();

  // Input is disabled when user can't post or websocket is disconnected.
  // Note: loading is intentionally excluded — the editor stays editable during sends
  // so users can type the next message while the current one is in flight.
  let inputDisabled = $derived(!canPost || connection().showConnectionLostBanner);

  let hasSendableAttachments = $derived(canAttach && attachments.selectedFiles.length > 0);

  // Can submit when there's content, not currently sending, and input is enabled.
  // hasVisibleContent rejects messages with only invisible Unicode characters.
  let canSubmit = $derived(
    !loading &&
      !inputDisabled &&
      attachments.pendingCount === 0 &&
      (hasVisibleContent(message) || hasSendableAttachments || isEditing)
  );
  let editorNextEnterWillSend = $state(false);
  let manualRichMode = $state(false);
  let editorHasRichStructure = $state(false);
  let isRichComposer = $derived(manualRichMode || editorHasRichStructure);
  let nextEnterWillSend = $derived(canSubmit && isRichComposer && editorNextEnterWillSend);
  let submitHint = $derived(
    shortcutHints && isRichComposer
      ? nextEnterWillSend
        ? shortcutHints.enterAgain
        : shortcutHints.submit
      : null
  );

  $effect(() => {
    if (!canAttach && attachments.filesWithUrls.length > 0) {
      attachments.clear();
    }
  });

  // Auto-focus the input when the component mounts, room changes, a reply
  // starts, or the editor becomes editable (canPost loads async after a
  // navigation, so on sidebar/quick-switcher room changes the editor is
  // briefly contenteditable=false — calling focus() then is a no-op).
  // Skip on touch devices where the keyboard popup would be jarring.
  $effect(() => {
    if (!autoFocus || !shouldAutoFocus()) return;

    // Tracked as dependencies so the effect re-fires on each of these.
    void roomId;
    void inReplyTo;

    if (editorApi && !inputDisabled) {
      tick().then(() => editorApi?.focus());
    }
  });

  // Handle emoji selection from autocomplete
  function handleEmojiSelect(emoji: string, _name: string) {
    autocomplete.selectEmoji(emoji);
  }

  function closeEmojiAutocomplete() {
    autocomplete.closeEmoji();
  }

  // Handle mention selection from autocomplete
  function handleMentionSelect(login: string, viaTab: boolean) {
    autocomplete.selectMention(login, viaTab);
  }

  function closeMentionAutocomplete() {
    autocomplete.closeMention();
  }

  function handleFileSelect(event: Event) {
    const target = event.target as HTMLInputElement;
    if (!canAttach) {
      target.value = '';
      return;
    }
    if (target.files) {
      void attachments.stageFiles(Array.from(target.files));
    }
    // Reset input so same file can be selected again
    target.value = '';
  }

  function removeFile(index: number) {
    attachments.removeFile(index);
  }

  /**
   * Add files from an external source (e.g., drag-and-drop).
   * Creates object URLs for preview and adds to the attachment list.
   */
  async function addFiles(files: File[]) {
    if (!canAttach) return;
    await attachments.stageFiles(files);
  }

  // Focus the input programmatically (e.g., when opening thread from mobile action sheet)
  function focus() {
    tick().then(() => editorApi?.focus());
  }

  function insertQuote(text: QuoteInsertionContent) {
    tick().then(() => editorApi?.insertQuote(text));
  }

  let insertedQuoteRequestId = 0;
  $effect(() => {
    const request = quoteInsertionState.request;
    const api = editorApi;
    if (!request || !api || request.id === insertedQuoteRequestId) return;

    insertedQuoteRequestId = request.id;
    api.insertQuote(request.text);
  });

  // Expose API to parent via onReady callback
  $effect(() => {
    onReady?.({ addFiles, focus, insertQuote });
  });

  // Handle paste events - intercept images before TipTap processes them
  function handlePaste(event: ClipboardEvent): boolean {
    // Don't accept file attachments in edit mode (editMessage only supports text)
    if (isEditing) return false;

    const items = event.clipboardData?.items;
    if (!items) return false;

    const pastedFiles: File[] = [];

    for (let i = 0; i < items.length; i++) {
      const item = items[i];
      if (item.type.startsWith('image/')) {
        const file = item.getAsFile();
        if (file) {
          pastedFiles.push(file);
        }
      }
    }

    if (pastedFiles.length > 0) {
      if (!canAttach) return true;
      void attachments.stageFiles(pastedFiles);
      return true; // Prevent TipTap from processing the paste
    }
    return false; // Let TipTap handle text pastes
  }

  // Collapse runs of 3+ newlines down to 2 (one blank line max).
  // Applied symmetrically on post and edit so blank-line runs don't
  // accumulate over time and pasted blank-line runs stay reasonable.
  function normalizeMessageBody(text: string): string {
    return text.replace(/\n{3,}/g, '\n\n');
  }

  function hasStructuralMarkdownBody(text: string): boolean {
    return text
      .split('\n')
      .some((line) => /^ {0,3}(?:#{1,6}|[-+*]|\d{1,9}[.)]|>)[ \t]$/.test(line));
  }

  function bodyForSend(text: string): string {
    const normalized = normalizeMessageBody(text);
    if (hasStructuralMarkdownBody(normalized)) return normalized;
    return normalizeMessageBody(text.trim());
  }

  type MentionConfirmation = {
    recipientCount: number;
    token: string;
  };

  type PreparedPost = {
    roomId: string;
    bodyToSend: string;
    filesToSend: File[] | null;
    threadRootEventId: string | null;
    inReplyTo: string | null;
    linkPreviewInput: ReturnType<typeof linkPreviews.buildInput>;
    alsoSendToChannel: boolean;
    wasRichComposer: boolean;
  };

  type PendingMentionConfirmation = PreparedPost & MentionConfirmation;

  type SendPreparedPostResponse = {
    event: RoomEventViewFragment | null;
    error: unknown | null;
    mentionConfirmation: MentionConfirmation | null;
  };

  let pendingMentionConfirmation = $state<PendingMentionConfirmation | null>(null);
  let mentionConfirmationLoading = $state(false);

  function mentionConfirmation(error: unknown): MentionConfirmation | null {
    const graphQLErrors =
      error && typeof error === 'object' && 'graphQLErrors' in error
        ? (error.graphQLErrors as Array<{ extensions?: Record<string, unknown> }>)
        : [];

    for (const graphQLError of graphQLErrors) {
      const extensions = graphQLError.extensions;
      if (extensions?.code !== 'MENTION_CONFIRMATION_REQUIRED') continue;
      const count = extensions.recipientCount;
      const token = extensions.mentionConfirmationToken;
      if (typeof count === 'number' && typeof token === 'string' && token) {
        return { recipientCount: count, token };
      }
    }
    return null;
  }

  function buildPostVariables(post: PreparedPost, mentionConfirmationToken: string | null) {
    return {
      input: {
        roomId: post.roomId,
        body: post.bodyToSend || null,
        attachments: post.filesToSend,
        threadRootEventId: post.threadRootEventId,
        inReplyTo: post.inReplyTo,
        linkPreview: post.linkPreviewInput,
        alsoSendToChannel: post.alsoSendToChannel || null,
        mentionConfirmationToken
      }
    };
  }

  async function sendPreparedPost(
    post: PreparedPost,
    mentionConfirmationToken: string | null
  ): Promise<SendPreparedPostResponse> {
    if (post.filesToSend) {
      const response = await connection().client.mutation(
        PostMessageMutation,
        buildPostVariables(post, mentionConfirmationToken)
      );
      return {
        event: response.data?.postMessage
          ? useFragment(RoomEventViewFragmentDoc, response.data.postMessage)
          : null,
        error: response.error ?? null,
        mentionConfirmation: response.error ? mentionConfirmation(response.error) : null
      };
    }

    try {
      const conn = connection();
      const result = await createMessageAPI({
        serverId: conn.serverId,
        baseUrl: conn.connectBaseUrl,
        bearerToken: conn.bearerToken
      }).postMessage({
        roomId: post.roomId,
        body: post.bodyToSend,
        threadRootEventId: post.threadRootEventId,
        inReplyTo: post.inReplyTo,
        linkPreview: post.linkPreviewInput,
        alsoSendToChannel: post.alsoSendToChannel,
        mentionConfirmationToken
      });

      if (result.kind === 'mentionConfirmation') {
        return {
          event: null,
          error: null,
          mentionConfirmation: {
            recipientCount: result.recipientCount,
            token: result.token
          }
        };
      }
      return { event: result.event, error: null, mentionConfirmation: null };
    } catch (error) {
      return { event: null, error, mentionConfirmation: null };
    }
  }

  function restorePreparedPost(post: PreparedPost) {
    autocomplete.reset();
    message = post.bodyToSend;
    manualRichMode = post.wasRichComposer;
    editorApi?.setContent(post.bodyToSend);
    if (post.filesToSend) {
      attachments.restore(attachments.filesToPreviewItems(post.filesToSend));
    }
  }

  function handlePostFailure(error: unknown, post: PreparedPost) {
    toast.error(m['composer.send_failed']());
    console.error('Error posting message:', error);
    restorePreparedPost(post);
  }

  function handlePostSuccess(response: SendPreparedPostResponse, post: PreparedPost) {
    // Notify parent before scrolling so it can synchronously ingest the
    // returned event and make the target row available.
    onMessageSent?.(response.event);

    // Scroll the enclosing pane to the user's new message. The composer
    // reads `scrollState` from its surrounding ComposerContext, so this
    // targets the main room's EventList in a room composer and the
    // thread's EventList in a thread composer.
    scrollState?.requestScrollToBottom();

    // Clear reply-in-room state after sending
    onCancelReply?.();

    // Mark this room as read (we just posted, so we've seen all messages)
    roomUnreadStore.setRoomUnread(post.roomId, false);

    // Reset "also send to channel" checkbox after successful send
    alsoSendToChannel = false;
    manualRichMode = false;
  }

  function cancelMentionConfirmation() {
    const pendingPost = pendingMentionConfirmation;
    pendingMentionConfirmation = null;
    if (pendingPost) {
      restorePreparedPost(pendingPost);
    }
  }

  async function confirmMentionSend() {
    const pendingPost = pendingMentionConfirmation;
    if (!pendingPost || mentionConfirmationLoading) return;

    mentionConfirmationLoading = true;
    try {
      const response = await sendPreparedPost(pendingPost, pendingPost.token);
      pendingMentionConfirmation = null;

      if (response.error) {
        handlePostFailure(response.error, pendingPost);
      } else if (response.mentionConfirmation) {
        pendingMentionConfirmation = { ...pendingPost, ...response.mentionConfirmation };
      } else {
        handlePostSuccess(response, pendingPost);
      }
    } finally {
      mentionConfirmationLoading = false;
    }
  }

  async function postMessage() {
    // Require either non-empty message body or attachments.
    // hasVisibleContent rejects messages with only invisible Unicode characters.
    const bodyToSend = bodyForSend(message);
    const hasBody = hasVisibleContent(bodyToSend);
    const filesToSend = hasSendableAttachments ? [...attachments.selectedFiles] : null;
    if (!hasBody && !filesToSend) return;

    const preparedPost: PreparedPost = {
      roomId,
      bodyToSend,
      filesToSend,
      threadRootEventId: inThread ?? null,
      inReplyTo: inReplyTo ?? null,
      linkPreviewInput: linkPreviews.buildInput(),
      alsoSendToChannel,
      wasRichComposer: isRichComposer
    };

    // Optimistically clear the editor so the user can start typing the next
    // message immediately (matches Slack/Discord behavior).
    autocomplete.reset();
    message = '';
    manualRichMode = false;
    editorApi?.setContent('');
    attachments.clear();
    linkPreviews.clear();

    loading = true;

    try {
      const response = await sendPreparedPost(preparedPost, null);

      if (response.mentionConfirmation) {
        pendingMentionConfirmation = { ...preparedPost, ...response.mentionConfirmation };
        return;
      }

      if (response.error) {
        handlePostFailure(response.error, preparedPost);
      } else {
        handlePostSuccess(response, preparedPost);
      }
    } finally {
      loading = false;
    }
  }

  async function editMessage() {
    const trimmedBody = bodyForSend(message);
    if (!trimmedBody) {
      toast.error('Message cannot be empty');
      return;
    }

    const eventId = editState.eventId;
    if (!eventId) return;

    loading = true;

    const input: {
      roomId: string;
      eventId: string;
      body: string;
      alsoSendToChannel?: boolean;
    } = { roomId, eventId, body: trimmedBody };
    if (showEditEchoCheckbox) {
      input.alsoSendToChannel = alsoSendToChannel;
    }

    const response = await connection().client.mutation(UpdateMessageMutation, {
      input
    });

    if (response.error) {
      toast.error(response.error.message || m['composer.edit_failed']());
    } else {
      autocomplete.reset();
      message = '';
      editorApi?.setContent('');
      editState.cancelEdit();
    }

    loading = false;
  }

  async function handleSubmit() {
    // Guard against double-sends while editor stays editable, and against
    // submitting before pasted/dropped/selected files have finished staging.
    if (loading || inputDisabled || attachments.pendingCount > 0) return;
    if (isEditing) {
      await editMessage();
    } else {
      await postMessage();
    }
  }

  function cancelEdit() {
    autocomplete.reset();
    editState.cancelEdit();
    message = '';
    manualRichMode = false;
    editorApi?.setContent('');
  }

  // Handle keyboard events from TipTap editor.
  // Return true to prevent TipTap's default handling.
  function handleEditorKeyDown(event: KeyboardEvent): boolean {
    // Handle emoji autocomplete keyboard events first
    if (autocomplete.emoji && autocomplete.emojiRef) {
      if (autocomplete.emojiRef.handleKeyDown(event)) {
        return true;
      }
    }

    // Handle mention autocomplete keyboard events
    if (autocomplete.mention && autocomplete.mentionRef) {
      if (autocomplete.mentionRef.handleKeyDown(event)) {
        return true;
      }
    }

    if (event.key === 'Enter' && !event.shiftKey) {
      if (event.metaKey || event.ctrlKey) {
        if (isRichComposer) {
          handleSubmit(); // Fire-and-forget (async, but keydown must return sync)
        } else {
          manualRichMode = true;
          editorApi?.insertBlockBreak();
        }
        return true;
      }

      if (!isRichComposer) {
        if (canSubmit) {
          handleSubmit(); // Fire-and-forget (async, but keydown must return sync)
          return true;
        }
      } else if (nextEnterWillSend) {
        handleSubmit(); // Fire-and-forget (async, but keydown must return sync)
        return true;
      }
    }

    // Handle Tab for @mention autocomplete
    if (event.key === 'Tab') {
      if (autocomplete.handleTabCompletion(event)) {
        return true;
      }
      // If no completion happened, let default Tab behavior occur
    }

    // Reset tab-completion state on any other key
    if (event.key !== 'Tab') {
      autocomplete.resetTabCompletion();
    }

    if (event.key === 'Escape') {
      if (isEditing) {
        cancelEdit();
        return true;
      }
      if (inReplyTo && onCancelReply) {
        onCancelReply();
        return true;
      }
      if (onEscape) {
        onEscape();
        return true;
      }
    }

    // Up arrow on empty input: edit last message
    if (event.key === 'ArrowUp' && !isEditing && (editorApi?.getText() ?? '').trim() === '') {
      const lastMsg = lastEditableMessageCtx?.getLastEditableMessage();
      if (lastMsg) {
        editState.startEdit(lastMsg.eventId, lastMsg.body, {
          threadRootEventId: lastMsg.threadRootEventId,
          channelEchoEventId: lastMsg.channelEchoEventId,
          canAddChannelEcho: lastMsg.canAddChannelEcho
        });
        return true;
      }
    }

    return false; // Let TipTap handle it (e.g., Shift+Enter for hard break)
  }

  // Handle content updates from TipTap editor
  function handleEditorUpdate(text: string) {
    const previousMessage = message;
    message = text;
    if (!text) {
      manualRichMode = false;
    }
    // Only trigger typing indicator for actual user input.
    // Programmatic setContent calls suppress TipTap update events, but this
    // guard still protects any same-value editor update from emitting typing.
    if (text !== previousMessage) {
      onTyping?.();
    }
    autocomplete.update();
  }

  function handleRichStructureChange(value: boolean) {
    editorHasRichStructure = value;
  }

  // Called when TipTap editor is ready - sync any pending state
  function handleEditorReady(api: TipTapEditorApi) {
    editorApi = api;
    // Sync current message state (may have draft loaded before editor was ready)
    if (message) {
      api.setContent(message);
    }
  }
</script>

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  {@attach observeComposerResize}
  class="flex flex-col gap-2 p-2"
  onclick={(e) => {
    if (!(e.target as HTMLElement).closest('button, a, input, label, select, .tiptap')) {
      editorApi?.focus();
    }
  }}
>
  <!-- Link / message preview -->
  {#if linkPreviews.activeURL}
    {@const url = linkPreviews.activeURL}
    {@const messageLink = parseMessageLink(url)}
    {#if messageLink}
      <MessagePreviewCard link={messageLink} onDismiss={() => linkPreviews.dismissPreview(url)} />
    {:else if linkPreviews.fetchingURLs.has(url)}
      <LinkPreviewSkeleton />
    {:else if linkPreviews.previews.get(url)}
      <LinkPreviewCard
        preview={linkPreviews.previews.get(url)!}
        onDismiss={() => linkPreviews.dismissPreview(url)}
      />
    {/if}
  {/if}

  <!-- Selected files preview -->
  {#if attachments.filesWithUrls.length > 0}
    <div class="flex flex-wrap gap-2 rounded-lg bg-surface-300 p-2">
      {#each attachments.filesWithUrls as { file, url }, index (url)}
        <div class="relative">
          {#if file.type.startsWith('image/')}
            <img src={url} alt={file.name} class="h-16 w-16 rounded-md object-cover" />
          {:else if file.type.startsWith('video/')}
            <!-- Browser renders the first frame as a thumbnail from the object URL -->
            <video
              data-testid="video-attachment-preview"
              src="{url}#t=0.1"
              preload="metadata"
              muted
              class="h-16 w-16 rounded-md object-cover"
            ></video>
          {:else if file.type.startsWith('audio/')}
            <div
              data-testid="audio-attachment-preview"
              class="flex h-16 w-16 items-center justify-center rounded-md bg-surface-200"
            >
              <span class="iconify text-lg text-muted uil--music"></span>
            </div>
          {:else}
            <div class="flex h-16 w-16 items-center justify-center rounded-md bg-surface-200">
              <span class="text-xs text-muted">{file.name.split('.').pop()}</span>
            </div>
          {/if}
          <button
            type="button"
            onclick={() => removeFile(index)}
            class="absolute -top-1 -right-1 flex h-5 w-5 items-center justify-center rounded-full bg-red-500 text-xs text-white hover:bg-red-600"
          >
            ×
          </button>
        </div>
      {/each}
    </div>
  {/if}

  <!-- Hidden file input -->
  {#if canAttach && !isEditing}
    <input
      bind:this={fileInputElement}
      type="file"
      accept={attachments.accept}
      multiple
      onchange={handleFileSelect}
      class="hidden"
    />
  {/if}

  <!-- Unified input container -->
  <div
    class={[
      'relative flex items-start gap-4 rounded-xl bg-surface py-2 pr-3',
      isEditing ? 'pl-3' : 'pl-2'
    ]}
    class:opacity-50={inputDisabled}
    class:sending={loading}
  >
    <!-- Emoji autocomplete popup -->
    {#if autocomplete.emoji}
      <EmojiAutocomplete
        bind:this={autocomplete.emojiRef}
        query={autocomplete.emoji.query}
        onSelect={handleEmojiSelect}
        onClose={closeEmojiAutocomplete}
      />
    {/if}

    <!-- Mention autocomplete popup -->
    {#if autocomplete.mention}
      <MentionAutocomplete
        bind:this={autocomplete.mentionRef}
        query={autocomplete.mention.query}
        members={mentionCandidateMembers}
        roles={mentionRoles}
        onSelect={handleMentionSelect}
        onClose={closeMentionAutocomplete}
      />
    {/if}
    <!-- Attachment button - hidden in edit mode (editMessage only supports text) -->
    {#if !isEditing && canAttach}
      <button
        type="button"
        onclick={() => fileInputElement?.click()}
        disabled={inputDisabled}
        class="flex h-8 w-11 shrink-0 cursor-pointer items-center justify-center rounded text-muted transition-colors duration-100 enabled:hover:text-text disabled:cursor-not-allowed"
        title={m['composer.attach_file']()}
      >
        <span class="iconify text-xl uil--image-upload"></span>
      </button>
    {/if}

    <!-- Text input (TipTap editor) -->
    {#await tipTapEditorModule}
      <div class="min-h-8 min-w-0 flex-1 py-1" aria-hidden="true"></div>
    {:then { default: TipTapEditor }}
      <TipTapEditor
        placeholder={currentPlaceholder}
        editable={!inputDisabled}
        autofocus={autoFocus && shouldAutoFocus()}
        {testid}
        onUpdate={handleEditorUpdate}
        onKeyDown={handleEditorKeyDown}
        onPaste={handlePaste}
        onNextEnterWillSendChange={(value) => (editorNextEnterWillSend = value)}
        onRichStructureChange={handleRichStructureChange}
        onReady={handleEditorReady}
      />
    {/await}

    <div class="flex h-8 shrink-0 items-center gap-2">
      {#if submitHint && canSubmit}
        <span
          aria-hidden="true"
          title={submitHint}
          class="px-0.5 text-xs leading-none font-medium whitespace-nowrap text-muted/75"
        >
          {submitHint}
        </span>
      {/if}

      <!-- Send button -->
      <button
        type="button"
        onpointerdown={(e) => e.preventDefault()}
        onclick={handleSubmit}
        disabled={!canSubmit}
        class="flex h-8 w-8 cursor-pointer items-center justify-center rounded text-muted transition-colors duration-100 enabled:hover:text-text disabled:cursor-not-allowed disabled:opacity-50"
        aria-label={m['composer.send']()}
        title={isRichComposer ? m['composer.send_ctrl_enter']() : m['composer.send_enter']()}
      >
        <span class="iconify text-xl uil--telegram-alt"></span>
      </button>
    </div>
  </div>

  <!-- Also send to channel checkbox (thread replies only, when permitted) -->
  {#if (showAlsoSendToChannel && !isEditing) || showEditEchoCheckbox}
    <label class="flex cursor-pointer items-center gap-2 px-3 text-sm text-muted">
      <input
        type="checkbox"
        bind:checked={alsoSendToChannel}
        disabled={inputDisabled}
        class="cursor-pointer accent-primary"
      />
      {m['composer.also_send_to_channel']()}
    </label>
  {/if}

  <!-- Reply indicator -->
  {#if inReplyTo && replyDisplayName}
    <div
      data-testid="reply-indicator"
      class="flex items-center justify-between rounded-md bg-surface-200 px-3 py-2 text-sm"
    >
      <span class="min-w-0 truncate text-text">
        {m['composer.replying_to']()} <strong>{replyDisplayName}</strong>
        {#if replyExcerpt}
          <span class="text-muted"> &mdash; {replyExcerpt}</span>
        {/if}
      </span>
      <!-- Desktop: clickable "Esc to cancel" -->
      <button
        type="button"
        onclick={() => onCancelReply?.()}
        class="hidden shrink-0 cursor-pointer items-center gap-1 text-muted transition-colors hover:text-text sm:flex"
      >
        <kbd class="rounded bg-surface-300 px-1.5 py-0.5 text-xs">Esc</kbd>
        {m['composer.esc_to_cancel']()}
      </button>
      <!-- Mobile: visible "Cancel" button -->
      <button
        type="button"
        onclick={() => onCancelReply?.()}
        class="shrink-0 cursor-pointer rounded bg-surface-300 px-2.5 py-1 text-xs font-medium text-text transition-colors hover:bg-surface-highlighted sm:hidden"
      >
        {m['common.cancel']()}
      </button>
    </div>
  {/if}

  <!-- Edit mode indicator -->
  {#if isEditing}
    <div class="flex items-center justify-between rounded-md bg-surface-200 px-3 py-2 text-sm">
      <span class="text-text">{m['composer.editing']()}</span>
      <!-- Desktop: clickable "Esc to cancel" -->
      <button
        type="button"
        onclick={cancelEdit}
        class="hidden cursor-pointer items-center gap-1 text-muted transition-colors hover:text-text sm:flex"
      >
        <kbd class="rounded bg-surface-300 px-1.5 py-0.5 text-xs">Esc</kbd>
        {m['composer.esc_to_cancel']()}
      </button>
      <!-- Mobile: visible "Cancel" button -->
      <button
        type="button"
        onclick={cancelEdit}
        class="cursor-pointer rounded bg-surface-300 px-2.5 py-1 text-xs font-medium text-text transition-colors hover:bg-surface-highlighted sm:hidden"
      >
        {m['common.cancel']()}
      </button>
    </div>
  {/if}
</div>

{#if pendingMentionConfirmation}
  <ConfirmDialog
    title={m['composer.notify_title']({ count: pendingMentionConfirmation.recipientCount })}
    tone="warning"
    actionLabel={m['composer.send_anyway']()}
    actionIcon="iconify uil--telegram-alt"
    loading={mentionConfirmationLoading}
    onconfirm={confirmMentionSend}
    onclose={cancelMentionConfirmation}
  >
    {m['composer.notify_body']({ count: pendingMentionConfirmation.recipientCount })}
  </ConfirmDialog>
{/if}

<style>
  .sending {
    position: relative;
    overflow: hidden;
    background: linear-gradient(
      90deg,
      var(--color-surface) 0%,
      var(--color-surface-highlighted) 50%,
      var(--color-surface) 100%
    );
    background-size: 200% 100%;
    animation: shimmer 1.5s ease-in-out infinite;
  }

  @keyframes shimmer {
    0% {
      background-position: 200% 0;
    }
    100% {
      background-position: -200% 0;
    }
  }
</style>
