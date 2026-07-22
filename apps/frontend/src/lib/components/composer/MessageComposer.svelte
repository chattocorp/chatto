<script lang="ts">
  import { onDestroy, tick, untrack } from 'svelte';
  import type { RoomEventView } from '$lib/render/types';
  import { createMessageAPI } from '$lib/api-client/messages';
  import { createLinkPreviewAPI } from '$lib/api-client/linkPreviews';
  import { createRoleAPI } from '$lib/api-client/roles';
  import * as m from '$lib/i18n/messages';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { parseMessageLink } from '$lib/messageLinks';
  import LinkPreviewCard from '$lib/components/LinkPreviewCard.svelte';
  import LinkPreviewSkeleton from '$lib/components/LinkPreviewSkeleton.svelte';
  import MessagePreviewCard from '$lib/components/MessagePreviewCard.svelte';
  import ConfirmDialog from '$lib/ui/ConfirmDialog.svelte';
  import ContextMenu from '$lib/ui/ContextMenu.svelte';
  import { toast } from '$lib/ui/toast';
  import {
    getRoomMembers,
    getRoomMembersStore,
    getComposerContext,
    type QuoteInsertionContent,
    type RoomMember
  } from '$lib/state/room';
  import { shouldAutoFocus } from '$lib/utils/shouldAutoFocus';
  import { prefersTouchActions } from '$lib/utils/inputCapabilities';
  import { hasVisibleContent } from '$lib/validation';
  import { extractMentions, hasRoleOrVirtualMention } from '$lib/mentions';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { getUserSettings } from '$lib/state/userSettings.svelte';
  import EmojiAutocomplete from '$lib/components/composer/EmojiAutocomplete.svelte';
  import MentionAutocomplete from '$lib/components/composer/MentionAutocomplete.svelte';
  import type {
    ComposerFormattingCommand,
    ComposerFormattingState,
    TipTapEditorApi
  } from './TipTapEditor.svelte';
  import { DraftState, draftKey } from './draft.svelte';
  import { AttachmentsState } from './attachments.svelte';
  import { LinkPreviewState } from './linkPreviews.svelte';
  import { AutocompleteState, type MentionRole } from './autocomplete.svelte';
  import {
    createMessageTimestampToken,
    dateToDatetimeLocalValue,
    localDatetimeToEpochSeconds
  } from '$lib/messageTimestamps';

  const tipTapEditorModule = import('./TipTapEditor.svelte');
  const timestampTimezoneListId = `timestamp-timezones-${Math.random().toString(36).slice(2)}`;
  const emptyFormattingState: ComposerFormattingState = {
    bold: false,
    italic: false,
    inlineCode: false,
    heading: false,
    bulletList: false,
    orderedList: false,
    blockquote: false,
    codeBlock: false
  };
  const formattingControls: {
    command: ComposerFormattingCommand;
    icon?: string;
  }[] = [
    { command: 'bold', icon: 'mdi--format-bold' },
    { command: 'italic', icon: 'mdi--format-italic' },
    { command: 'inlineCode', icon: 'mdi--code-tags' },
    { command: 'heading', icon: 'mdi--format-header-2' },
    { command: 'bulletList', icon: 'mdi--format-list-bulleted' },
    { command: 'orderedList', icon: 'mdi--format-list-numbered' },
    { command: 'blockquote', icon: 'mdi--format-quote-open' },
    { command: 'codeBlock', icon: 'mdi--code-block-braces' }
  ];

  type ShortcutHints = {
    submit: string;
    enterAgain: string;
  };

  function getShortcutHints(): ShortcutHints | null {
    if (typeof navigator === 'undefined' || prefersTouchActions()) return null;

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
    onMessageSent?: (event: RoomEventView | null) => void;
    onCancelReply?: () => void;
    onEscape?: () => void;
    showAlsoSendToChannel?: boolean;
  } = $props();

  const connection = useConnection();
  const userSettings = getUserSettings();

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
  const linkPreviews = new LinkPreviewState(() => {
    const conn = connection();
    return createLinkPreviewAPI({
      serverId: conn.serverId,
      baseUrl: conn.connectBaseUrl,
      bearerToken: conn.bearerToken
    });
  });
  const autocomplete = new AutocompleteState(
    () => editorApi,
    () => mentionCandidateMembers,
    () => mentionRoles
  );
  let mentionRoles = $state<MentionRole[]>([]);
  let mentionRolesLoadComplete = $state(false);
  let mentionRolesLoadFailed = $state(false);
  let mentionRolesLoadPromise: Promise<boolean> | null = null;

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

  $effect(() => {
    return linkPreviews.scheduleDetection(message, isEditing);
  });

  $effect(() => {
    const conn = connection();
    const api = createRoleAPI({
      baseUrl: conn.connectBaseUrl,
      bearerToken: conn.bearerToken
    });
    let cancelled = false;
    mentionRoles = [];
    mentionRolesLoadComplete = false;
    mentionRolesLoadFailed = false;

    async function loadMentionRoles() {
      let roles;
      try {
        roles = (await api.listRoles()).roles;
      } catch {
        if (!cancelled) {
          mentionRoles = [];
          mentionRolesLoadFailed = true;
          mentionRolesLoadComplete = true;
        }
        return false;
      }
      if (cancelled) return false;
      mentionRoles =
        roles.map((role) => ({
          name: role.name,
          isSystem: role.isSystem,
          position: role.position,
          pingable: role.pingable
        })) ?? [];
      mentionRolesLoadFailed = false;
      mentionRolesLoadComplete = true;
      return true;
    }

    mentionRolesLoadPromise = loadMentionRoles();
    return () => {
      cancelled = true;
    };
  });

  let loading = $state(false);
  let roleMentionCheckLoading = $state(false);
  let fileInputElement = $state<HTMLInputElement>();
  let timestampTriggerElement = $state<HTMLButtonElement>();
  let timestampDateTimeInput = $state<HTMLInputElement>();
  let formattingState = $state<ComposerFormattingState>({ ...emptyFormattingState });
  let timestampPickerOpen = $state(false);
  let timestampPickerAnchor = $state<{ top: number; bottom: number; left: number } | null>(null);
  let timestampLocalValue = $state('');
  let timestampTimezoneSearch = $state('');
  const timestampTimezoneOptions = Intl.supportedValuesOf?.('timeZone') ?? [];
  const timestampTimezoneSuggestions = $derived(
    timestampTimezoneOptions
      .filter((timezone) =>
        timezone.toLowerCase().includes(timestampTimezoneSearch.trim().toLowerCase())
      )
      .slice(0, 60)
  );
  const timestampTimeZoneValid = $derived(isValidTimestampTimeZone(timestampTimezoneSearch));
  const timestampEpochSeconds = $derived(
    timestampTimeZoneValid
      ? localDatetimeToEpochSeconds(timestampLocalValue, timestampTimezoneSearch.trim())
      : null
  );
  const timestampPickerError = $derived.by(() => {
    if (!timestampLocalValue) return m['composer.timestamp.error_required']();
    if (!timestampTimeZoneValid) return m['composer.timestamp.error_timezone']();
    if (timestampEpochSeconds === null) return m['composer.timestamp.error_invalid']();
    return null;
  });

  // Input is disabled when user can't post or websocket is disconnected.
  // Note: loading is intentionally excluded — the editor stays editable during sends
  // so users can type the next message while the current one is in flight.
  let inputDisabled = $derived(!canPost || connection().showConnectionLostBanner);

  let hasSendableAttachments = $derived(canAttach && attachments.selectedFiles.length > 0);

  // Can submit when there's content, not currently sending, and input is enabled.
  // hasVisibleContent rejects messages with only invisible Unicode characters.
  let canSubmit = $derived(
    !loading &&
      !roleMentionCheckLoading &&
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

  function formattingLabel(command: ComposerFormattingCommand): string {
    switch (command) {
      case 'bold':
        return m['composer.format.bold']();
      case 'italic':
        return m['composer.format.italic']();
      case 'inlineCode':
        return m['composer.format.inline_code']();
      case 'heading':
        return m['composer.format.heading']();
      case 'bulletList':
        return m['composer.format.bullet_list']();
      case 'orderedList':
        return m['composer.format.ordered_list']();
      case 'blockquote':
        return m['composer.format.blockquote']();
      case 'codeBlock':
        return m['composer.format.code_block']();
    }
  }

  function toggleFormatting(command: ComposerFormattingCommand) {
    editorApi?.toggleFormatting(command);
  }

  function browserTimeZone(): string {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
  }

  function preferredTimestampTimeZone(): string {
    const timezone = userSettings.effectiveTimezone ?? browserTimeZone();
    return isValidTimestampTimeZone(timezone) ? timezone : 'UTC';
  }

  function isValidTimestampTimeZone(timezone: string): boolean {
    const trimmed = timezone.trim();
    if (!trimmed) return false;
    try {
      new Intl.DateTimeFormat('en-US', { timeZone: trimmed }).format(new Date());
      return true;
    } catch {
      return false;
    }
  }

  function openTimestampPicker(event: MouseEvent) {
    if (inputDisabled) return;
    const button = event.currentTarget as HTMLButtonElement;
    const rect = button.getBoundingClientRect();
    const timezone = preferredTimestampTimeZone();
    timestampTriggerElement = button;
    timestampTimezoneSearch = timezone;
    timestampLocalValue = dateToDatetimeLocalValue(new Date(Date.now() + 60 * 60_000), timezone);
    timestampPickerAnchor = { top: rect.top, bottom: rect.bottom, left: rect.left };
    timestampPickerOpen = true;
    tick().then(() => {
      if (!timestampPickerOpen) return;
      timestampDateTimeInput?.focus();
      timestampDateTimeInput?.select();
    });
  }

  function closeTimestampPicker({ restoreFocus = true }: { restoreFocus?: boolean } = {}) {
    timestampPickerOpen = false;
    timestampPickerAnchor = null;
    if (restoreFocus) {
      timestampTriggerElement?.focus();
    }
  }

  function insertTimestamp(event: SubmitEvent) {
    event.preventDefault();
    const epochSeconds = timestampEpochSeconds;
    if (epochSeconds === null || !editorApi) return;

    const token = createMessageTimestampToken(epochSeconds);
    const beforeCursor = editorApi.getTextBeforeCursor();
    const prefix = beforeCursor.length > 0 && !/\s$/.test(beforeCursor) ? ' ' : '';
    editorApi.insertText(`${prefix}${token} `);
    closeTimestampPicker({ restoreFocus: false });
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

  type PreparedPost = {
    roomId: string;
    bodyToSend: string;
    filesToSend: File[] | null;
    attachmentAssetIds?: string[];
    threadRootEventId: string | null;
    inReplyTo: string | null;
    linkPreviewInput: ReturnType<typeof linkPreviews.buildInput>;
    alsoSendToChannel: boolean;
    wasRichComposer: boolean;
  };

  type SendPreparedPostResponse = {
    event: RoomEventView | null;
    error: unknown | null;
  };

  let pendingRoleMentionConfirmation = $state<PreparedPost | null>(null);
  let roleMentionConfirmationLoading = $state(false);

  async function ensureMentionRolesLoadedForConfirmation(): Promise<boolean> {
    if (mentionRolesLoadComplete) return !mentionRolesLoadFailed;
    return (await mentionRolesLoadPromise) ?? false;
  }

  function postMentionsRoleOrVirtualTarget(post: PreparedPost, rolesAvailable: boolean): boolean {
    const hasKnownRoleOrVirtualMention = hasRoleOrVirtualMention(
      post.bodyToSend,
      mentionRoles.filter((role) => role.name !== 'everyone').map((role) => role.name)
    );
    if (hasKnownRoleOrVirtualMention) return true;
    if (rolesAvailable) return false;

    return extractMentions(post.bodyToSend).length > 0;
  }

  async function sendPreparedPost(post: PreparedPost): Promise<SendPreparedPostResponse> {
    try {
      const conn = connection();
      const result = await createMessageAPI({
        serverId: conn.serverId,
        baseUrl: conn.connectBaseUrl,
        bearerToken: conn.bearerToken
      }).createMessage({
        roomId: post.roomId,
        body: post.bodyToSend,
        attachmentAssetIds: post.attachmentAssetIds,
        attachments: post.attachmentAssetIds?.length ? null : post.filesToSend,
        threadRootEventId: post.threadRootEventId,
        inReplyTo: post.inReplyTo,
        linkPreview: post.linkPreviewInput,
        alsoSendToChannel: post.alsoSendToChannel
      });

      return { event: result.event, error: null };
    } catch (error) {
      return { event: null, error };
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
    console.error('Error creating message:', error);
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

  async function submitPreparedPost(preparedPost: PreparedPost) {
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
      const response = await sendPreparedPost(preparedPost);

      if (response.error) {
        handlePostFailure(response.error, preparedPost);
      } else {
        handlePostSuccess(response, preparedPost);
      }
    } finally {
      loading = false;
    }
  }

  function cancelRoleMentionConfirmation() {
    pendingRoleMentionConfirmation = null;
  }

  async function confirmRoleMentionSend() {
    const pendingPost = pendingRoleMentionConfirmation;
    if (!pendingPost || roleMentionConfirmationLoading) return;

    roleMentionConfirmationLoading = true;
    try {
      await submitPreparedPost(pendingPost);
      pendingRoleMentionConfirmation = null;
    } finally {
      roleMentionConfirmationLoading = false;
    }
  }

  async function createMessage() {
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

    let rolesAvailable = mentionRolesLoadComplete && !mentionRolesLoadFailed;
    if (hasBody && bodyToSend.includes('@') && !mentionRolesLoadComplete) {
      roleMentionCheckLoading = true;
      try {
        rolesAvailable = await ensureMentionRolesLoadedForConfirmation();
      } finally {
        roleMentionCheckLoading = false;
      }
    }

    if (hasBody && postMentionsRoleOrVirtualTarget(preparedPost, rolesAvailable)) {
      pendingRoleMentionConfirmation = preparedPost;
      return;
    }

    await submitPreparedPost(preparedPost);
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

    try {
      const conn = connection();
      await createMessageAPI({
        serverId: conn.serverId,
        baseUrl: conn.connectBaseUrl,
        bearerToken: conn.bearerToken
      }).updateMessage(input);
      autocomplete.reset();
      message = '';
      editorApi?.setContent('');
      editState.cancelEdit();
    } catch (error) {
      toast.error(error instanceof Error ? error.message : m['composer.edit_failed']());
    }

    loading = false;
  }

  async function handleSubmit() {
    // Guard against double-sends while editor stays editable, and against
    // submitting before pasted/dropped/selected files have finished staging.
    if (
      loading ||
      roleMentionCheckLoading ||
      roleMentionConfirmationLoading ||
      pendingRoleMentionConfirmation ||
      inputDisabled ||
      attachments.pendingCount > 0
    )
      return;
    if (isEditing) {
      await editMessage();
    } else {
      await createMessage();
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

    if (event.key === 'Enter' && !event.ctrlKey && !event.metaKey && prefersTouchActions()) {
      return false;
    }

    if (event.key === 'Enter' && !event.shiftKey) {
      if (event.metaKey || event.ctrlKey) {
        if (isRichComposer) {
          handleSubmit(); // Fire-and-forget (async, but keydown must return sync)
        } else {
          if (hasVisibleContent(message)) {
            editorApi?.insertBlockBreak();
          }
          // TipTap reports an empty document while inserting the first block break,
          // so commit manual rich mode after that update has had a chance to clear it.
          manualRichMode = true;
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
    <div class="flex flex-wrap gap-2 rounded-lg bg-surface-strong p-2">
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
              class="flex h-16 w-16 items-center justify-center rounded-md bg-surface-emphasized"
            >
              <span class="iconify text-lg text-muted uil--music"></span>
            </div>
          {:else}
            <div
              data-testid="file-attachment-preview"
              class="flex h-16 w-16 items-center justify-center rounded-md bg-surface-emphasized"
            >
              <span class="text-xs text-muted">{file.name.split('.').pop()}</span>
            </div>
          {/if}
          <button
            type="button"
            onclick={() => removeFile(index)}
            class="absolute -top-1 -right-1 flex h-5 w-5 items-center justify-center rounded-full bg-danger text-xs text-on-danger transition-[background-color] hover:bg-danger/80"
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
      multiple
      onchange={handleFileSelect}
      class="hidden"
    />
  {/if}

  <!-- Unified composer surface -->
  <div
    data-testid="composer-input-surface"
    data-composer-mode={isRichComposer ? 'rich' : 'simple'}
    class="composer-mode-surface relative flex flex-col rounded-lg bg-surface px-2.5 py-1.5"
    class:opacity-50={inputDisabled}
    class:composer-sending={loading}
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
    {#if timestampPickerOpen}
      <ContextMenu
        anchor={timestampPickerAnchor}
        role="dialog"
        ariaLabel={m['composer.timestamp.title']()}
        class="w-[min(22rem,calc(100vw-1rem))]"
        onclose={closeTimestampPicker}
      >
        <form class="flex flex-col gap-1" onsubmit={insertTimestamp}>
          <header class="flex items-center gap-2 menu-section px-3 py-2 text-sm font-medium">
            <span class="iconify uil--clock text-muted"></span>
            <span>{m['composer.timestamp.title']()}</span>
          </header>

          <section class="flex flex-col gap-3 menu-section px-3 py-2">
            <label class="flex flex-col gap-1 text-sm">
              <span class="text-muted">{m['composer.timestamp.date_time']()}</span>
              <input
                class="input"
                type="datetime-local"
                bind:this={timestampDateTimeInput}
                bind:value={timestampLocalValue}
                required
              />
            </label>

            <label class="flex flex-col gap-1 text-sm">
              <span class="text-muted">{m['composer.timestamp.timezone']()}</span>
              <input
                class="input"
                list={timestampTimezoneListId}
                bind:value={timestampTimezoneSearch}
                autocomplete="off"
                spellcheck="false"
                required
              />
              <datalist id={timestampTimezoneListId}>
                {#each timestampTimezoneSuggestions as timezone (timezone)}
                  <option value={timezone}></option>
                {/each}
              </datalist>
            </label>

            {#if timestampPickerError}
              <p class="form-error text-xs">{timestampPickerError}</p>
            {/if}
          </section>

          <footer class="flex justify-end gap-2 menu-section px-3 py-2">
            <button type="button" class="btn-secondary btn-sm" onclick={() => closeTimestampPicker()}>
              {m['common.cancel']()}
            </button>
            <button type="submit" class="btn-action btn-sm" disabled={timestampPickerError !== null}>
              {m['composer.timestamp.insert']()}
            </button>
          </footer>
        </form>
      </ContextMenu>
    {/if}
    <!-- Text input (TipTap editor) -->
    <div class="min-h-9 min-w-0 px-0.5 py-0.5" data-testid="composer-editor-row">
      {#await tipTapEditorModule}
        <div class="min-h-8 min-w-0" aria-hidden="true"></div>
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
          onFormattingStateChange={(state) => (formattingState = { ...state })}
          onReady={handleEditorReady}
        />
      {/await}
    </div>

    <div
      class="mt-0 flex min-h-7 items-center justify-between gap-2 border-t border-border/60 pt-0.5"
      data-testid="composer-toolbar"
    >
      <div class="flex items-center gap-1">
        <div
          class="flex min-w-0 flex-wrap items-center gap-0.5"
          data-testid="composer-formatting-toolbar"
        >
          {#each formattingControls as control (control.command)}
            {@const label = formattingLabel(control.command)}
            {@const active = formattingState[control.command]}
            <button
              type="button"
              onpointerdown={(event) => event.preventDefault()}
              onclick={() => toggleFormatting(control.command)}
              disabled={inputDisabled || !editorApi}
              aria-label={label}
              aria-pressed={active}
              title={label}
              class={[
                'flex h-6 w-6 cursor-pointer items-center justify-center rounded transition-[background-color,color,scale] duration-100 active:scale-[0.96] disabled:cursor-not-allowed disabled:opacity-50',
                active
                  ? 'bg-surface-emphasized text-text'
                  : 'text-muted enabled:hover:bg-surface-emphasized enabled:hover:text-text'
              ]}
            >
              <span class={['iconify text-[15px]', control.icon]}></span>
            </button>
          {/each}
        </div>

        <div class="mx-1 h-4 w-px bg-border/60"></div>

        <!-- Attachment button - hidden in edit mode (editMessage only supports text) -->
        {#if !isEditing && canAttach}
          <button
            type="button"
            onclick={() => fileInputElement?.click()}
            disabled={inputDisabled}
            class="flex h-6 w-6 shrink-0 cursor-pointer items-center justify-center rounded text-muted transition-[color,scale] duration-100 active:scale-[0.96] enabled:hover:bg-surface-emphasized enabled:hover:text-text disabled:cursor-not-allowed disabled:opacity-50"
            aria-label={m['composer.attach_file']()}
            title={m['composer.attach_file']()}
          >
            <span class="iconify text-[15px] uil--image-upload"></span>
          </button>
        {/if}

        <button
          type="button"
          onpointerdown={(e) => e.preventDefault()}
          onclick={openTimestampPicker}
          bind:this={timestampTriggerElement}
          disabled={inputDisabled}
          class="flex h-6 w-6 cursor-pointer items-center justify-center rounded text-muted transition-[background-color,color,scale] duration-100 active:scale-[0.96] enabled:hover:bg-surface-emphasized enabled:hover:text-text disabled:cursor-not-allowed disabled:opacity-50"
          aria-label={m['composer.timestamp.insert_label']()}
          title={m['composer.timestamp.insert_label']()}
        >
          <span class="iconify text-[15px] uil--clock"></span>
        </button>
      </div>

      <div class="flex items-center gap-2">
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
          class="flex h-6 w-6 cursor-pointer items-center justify-center rounded text-muted transition-[background-color,color,scale] duration-100 active:scale-[0.96] enabled:hover:bg-surface-emphasized enabled:hover:text-text disabled:cursor-not-allowed disabled:opacity-50"
          aria-label={m['composer.send']()}
          title={isRichComposer ? m['composer.send_ctrl_enter']() : m['composer.send_enter']()}
        >
          <span class="iconify text-[15px] uil--telegram-alt"></span>
        </button>
      </div>
    </div>
  </div>

  <!-- Also send to channel checkbox (thread replies only, when permitted) -->
  {#if (showAlsoSendToChannel && !isEditing) || showEditEchoCheckbox}
    <label class="flex cursor-pointer items-center gap-2 px-3 text-sm text-muted">
      <input
        type="checkbox"
        bind:checked={alsoSendToChannel}
        disabled={inputDisabled}
        class="cursor-pointer accent-neutral-action"
      />
      {m['composer.also_send_to_channel']()}
    </label>
  {/if}

  <!-- Reply indicator -->
  {#if inReplyTo && replyDisplayName}
    <div
      data-testid="reply-indicator"
      class="flex items-center justify-between rounded-md bg-surface-emphasized px-3 py-2 text-sm"
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
        <kbd class="rounded bg-surface-strong px-1.5 py-0.5 text-xs">Esc</kbd>
        {m['composer.esc_to_cancel']()}
      </button>
      <!-- Mobile: visible "Cancel" button -->
      <button
        type="button"
        onclick={() => onCancelReply?.()}
        class="shrink-0 cursor-pointer rounded bg-surface-strong px-2.5 py-1 text-xs font-medium text-text transition-colors hover:bg-surface-selected sm:hidden"
      >
        {m['common.cancel']()}
      </button>
    </div>
  {/if}

  <!-- Edit mode indicator -->
  {#if isEditing}
    <div
      class="flex items-center justify-between rounded-md bg-surface-emphasized px-3 py-2 text-sm"
    >
      <span class="text-text">{m['composer.editing']()}</span>
      <!-- Desktop: clickable "Esc to cancel" -->
      <button
        type="button"
        onclick={cancelEdit}
        class="hidden cursor-pointer items-center gap-1 text-muted transition-colors hover:text-text sm:flex"
      >
        <kbd class="rounded bg-surface-strong px-1.5 py-0.5 text-xs">Esc</kbd>
        {m['composer.esc_to_cancel']()}
      </button>
      <!-- Mobile: visible "Cancel" button -->
      <button
        type="button"
        onclick={cancelEdit}
        class="cursor-pointer rounded bg-surface-strong px-2.5 py-1 text-xs font-medium text-text transition-colors hover:bg-surface-selected sm:hidden"
      >
        {m['common.cancel']()}
      </button>
    </div>
  {/if}
</div>

{#if pendingRoleMentionConfirmation}
  <ConfirmDialog
    title={m['composer.role_mention_confirm_title']()}
    tone="warning"
    actionLabel={m['composer.send_anyway']()}
    actionIcon="iconify uil--telegram-alt"
    loading={roleMentionConfirmationLoading}
    onconfirm={confirmRoleMentionSend}
    onclose={cancelRoleMentionConfirmation}
  >
    {m['composer.role_mention_confirm_body']()}
  </ConfirmDialog>
{/if}
