<script module lang="ts">
  // Module-level Map to stash file attachments across room switches.
  // Files can't go in sessionStorage (not serializable), but this Map
  // survives component re-mounts within the same SPA session.
  type FileWithUrl = { file: File; url: string };
  // eslint-disable-next-line svelte/prefer-svelte-reactivity -- module-level stash, never read reactively
  const draftFilesMap = new Map<string, FileWithUrl[]>();
</script>

<script lang="ts">
  import { tick, untrack } from 'svelte';
  import { SvelteMap, SvelteSet } from 'svelte/reactivity';
  import { graphql } from '$lib/gql';
  import type { LinkPreviewForComposerQuery } from '$lib/gql/graphql';

  type ComposerLinkPreview = NonNullable<LinkPreviewForComposerQuery['linkPreview']>;
  import { useConnection } from '$lib/state/instance/connection.svelte';
  import { instanceRegistry } from '$lib/state/instance/registry.svelte';
  import { extractURLs } from '$lib/linkPreview';
  import { parseMessageLink } from '$lib/messageLinks';
  import LinkPreviewCard, { LinkPreviewFragment } from '$lib/components/LinkPreviewCard.svelte';
  import { useFragment } from '$lib/gql/fragment-masking';
  import LinkPreviewSkeleton from '$lib/components/LinkPreviewSkeleton.svelte';
  import MessagePreviewCard from '$lib/components/MessagePreviewCard.svelte';
  import { toast } from '$lib/ui/toast';
  import { getRoomMembers, getComposerContext } from '$lib/state/room';
  import { shouldAutoFocus } from '$lib/utils/shouldAutoFocus';
  import { isTouchDevice } from '$lib/utils/isTouchDevice';
  import { hasVisibleContent } from '$lib/validation';
  import { fuzzyMatch } from '$lib/fuzzyMatch';
  import { getActiveInstance } from '$lib/state/activeInstance.svelte';
  import { searchEmojis } from '$lib/emoji';
  import EmojiAutocomplete from '$lib/components/composer/EmojiAutocomplete.svelte';
  import MentionAutocomplete from '$lib/components/composer/MentionAutocomplete.svelte';
  import TipTapEditor, { type TipTapEditorApi } from './TipTapEditor.svelte';
  import { prepareFiles } from '$lib/attachments/prepareFiles';

  const getInstanceId = getActiveInstance();
  const stores = instanceRegistry.getStore(getInstanceId());
  const instanceState = stores.instance;
  const roomUnreadStore = stores.roomUnread;

  export type MessageComposerApi = {
    addFiles: (files: File[]) => void;
    focus: () => void;
  };

  let {
    spaceId,
    roomId,
    inThread,
    inReplyTo,
    replyDisplayName,
    replyExcerpt,
    placeholder: customPlaceholder,
    canPost = true,
    autoFocus = true,
    onReady,
    onTyping,
    onMessageSent,
    onCancelReply,
    onEscape,
    showAlsoSendToChannel = false
  }: {
    spaceId: string;
    roomId: string;
    inThread?: string;
    inReplyTo?: string;
    replyDisplayName?: string;
    replyExcerpt?: string;
    placeholder?: string;
    canPost?: boolean;
    autoFocus?: boolean;
    onReady?: (api: MessageComposerApi) => void;
    onTyping?: () => void;
    onMessageSent?: () => void;
    onCancelReply?: () => void;
    onEscape?: () => void;
    showAlsoSendToChannel?: boolean;
  } = $props();

  const connection = useConnection();

  let alsoSendToChannel = $state(false);

  // Get room members from context (provided by Room.svelte)
  const members = $derived(getRoomMembers());

  const composerContext = getComposerContext();
  const editState = composerContext.editState;
  const lastEditableMessageCtx = composerContext.lastEditableMessage;
  const scrollState = composerContext.scrollState;
  const isEditing = $derived(editState.eventId !== null);

  // Element ref for ResizeObserver
  let composerEl = $state<HTMLDivElement>();

  // When the composer resizes (editor grows/shrinks, attachments added/removed),
  // scroll to bottom if sticky. This replaces the synchronous scrollToBottomIfSticky()
  // that was lost when the old textarea's autoResize() was removed during TipTap migration.
  $effect(() => {
    if (!composerEl || !scrollState) return;

    const observer = new ResizeObserver(() => {
      scrollState.scrollToBottomIfSticky();
    });
    observer.observe(composerEl);
    return () => observer.disconnect();
  });

  const DRAFT_KEY = $derived(
    inThread ? `chatto:draft:${roomId}:thread:${inThread}` : `chatto:draft:${roomId}`
  );
  let message = $state('');

  // TipTap editor API (received via onReady callback)
  let editorApi = $state<TipTapEditorApi | null>(null);

  // Dynamic placeholder changes between normal and edit mode
  let currentPlaceholder = $derived(
    isEditing ? 'Editing message...' : (customPlaceholder ?? 'Type a message...')
  );

  // Testid for E2E tests - distinguishes main input from thread reply input
  let testid = $derived(inThread ? 'thread-reply-input' : 'message-input');

  // Track previous editing state to detect transitions
  let wasEditing = $state(false);

  // When entering edit mode, pre-fill with original message body and clear any pending attachments.
  // When exiting edit mode (cancelled or message deleted), clear the input.
  $effect(() => {
    if (editState.eventId && editState.originalBody) {
      message = editState.originalBody;
      wasEditing = true;
      editorApi?.setContent(editState.originalBody);
      tick().then(() => editorApi?.focus('end'));

      // Clear pending file attachments — editMessage only supports text.
      // Use untrack() to avoid making filesWithUrls a dependency of this effect,
      // which would cause re-runs that overwrite the message with originalBody.
      untrack(() => {
        for (const { url } of filesWithUrls) {
          URL.revokeObjectURL(url);
        }
        filesWithUrls = [];
      });
    } else if (wasEditing && !editState.eventId) {
      // Exiting edit mode - clear the input
      message = '';
      wasEditing = false;
      editorApi?.setContent('');
    }
  });

  // Load draft from sessionStorage when room changes
  // Using sessionStorage (not localStorage) so drafts are tab-specific
  $effect(() => {
    const draft = sessionStorage.getItem(DRAFT_KEY) ?? '';
    message = draft;
    editorApi?.setContent(draft);
  });

  // Persist draft to sessionStorage
  $effect(() => {
    if (message) {
      sessionStorage.setItem(DRAFT_KEY, message);
    } else {
      sessionStorage.removeItem(DRAFT_KEY);
    }
  });

  const PostMessageMutation = graphql(`
    mutation PostMessage($input: PostMessageInput!) {
      postMessage(input: $input) {
        id
      }
    }
  `);

  const EditMessageMutation = graphql(`
    mutation EditMessageFromInput($input: EditMessageInput!) {
      editMessage(input: $input)
    }
  `);

  const LinkPreviewQuery = graphql(`
    query LinkPreviewForComposer($url: String!) {
      linkPreview(url: $url) {
        ...LinkPreviewView
        imageAssetId
      }
    }
  `);

  // Link preview state
  let detectedURLs = $state<string[]>([]);
  const previews = new SvelteMap<string, ComposerLinkPreview | null>();
  const dismissedURLs = new SvelteSet<string>();
  const fetchingURLs = new SvelteSet<string>();

  // Debounced URL detection (500ms)
  let urlDetectionTimeout: ReturnType<typeof setTimeout>;

  $effect(() => {
    // Track message for reactivity
    const currentMessage = message;

    // Clear previous timeout
    clearTimeout(urlDetectionTimeout);

    // Don't detect URLs in edit mode
    if (isEditing) {
      detectedURLs = [];
      return;
    }

    urlDetectionTimeout = setTimeout(() => {
      const urls = extractURLs(currentMessage).filter((u) => !dismissedURLs.has(u));
      detectedURLs = urls;

      // Fetch OG previews for new URLs (skip message links — those are rendered from a separate GraphQL query)
      for (const url of urls) {
        if (parseMessageLink(url)) continue;
        if (!previews.has(url) && !fetchingURLs.has(url)) {
          fetchPreview(url);
        }
      }
    }, 500);

    return () => clearTimeout(urlDetectionTimeout);
  });

  async function fetchPreview(url: string) {
    fetchingURLs.add(url);

    const result = await connection().client.query(LinkPreviewQuery, { url });

    fetchingURLs.delete(url);

    if (result.data?.linkPreview) {
      previews.set(url, result.data.linkPreview);
    } else {
      // Mark as fetched but no preview available
      previews.set(url, null);
    }
  }

  function dismissPreview(url: string) {
    dismissedURLs.add(url);
    detectedURLs = detectedURLs.filter((u) => u !== url);
  }

  // Clear link preview state when message is sent
  function clearLinkPreviews() {
    detectedURLs = [];
    previews.clear();
    dismissedURLs.clear();
    fetchingURLs.clear();
  }

  let loading = $state(false);
  let fileInputElement = $state<HTMLInputElement>();

  let filesWithUrls = $state<FileWithUrl[]>([]);

  // Derive just the files for posting
  let selectedFiles = $derived(filesWithUrls.map((f) => f.file));

  // Save/restore draft file attachments across room switches.
  // When leaving a room (DRAFT_KEY changes or unmount), stash files in the
  // module-level Map. When entering a room, restore any stashed files.
  $effect(() => {
    const key = DRAFT_KEY;

    const saved = draftFilesMap.get(key);
    if (saved) {
      filesWithUrls = saved;
      draftFilesMap.delete(key);
    } else {
      filesWithUrls = [];
    }

    return () => {
      if (filesWithUrls.length > 0) {
        draftFilesMap.set(key, filesWithUrls);
      } else {
        draftFilesMap.delete(key);
      }
    };
  });

  // Input is disabled when user can't post or websocket is disconnected.
  // Note: loading is intentionally excluded — the editor stays editable during sends
  // so users can type the next message while the current one is in flight.
  let inputDisabled = $derived(!canPost || connection().showConnectionLostBanner);

  // Can submit when there's content, not currently sending, and input is enabled.
  // hasVisibleContent rejects messages with only invisible Unicode characters.
  let canSubmit = $derived(
    !loading &&
      !inputDisabled &&
      (hasVisibleContent(message) || selectedFiles.length > 0 || isEditing)
  );

  // Auto-focus the input when the component mounts, room changes, or a reply starts.
  // Skip on touch devices where keyboard popup would be jarring.
  $effect(() => {
    if (!autoFocus || !shouldAutoFocus()) return;

    // Track roomId, inReplyTo, and editorApi as dependencies
    void roomId;
    void inReplyTo;

    if (editorApi) {
      tick().then(() => editorApi?.focus());
    }
  });

  // Tab-completion state for @mentions
  type TabCompletionState = {
    candidates: string[]; // Matching usernames
    index: number; // Current position in cycle
    triggerStart: number; // Position of @ in text (relative to full text before cursor)
    originalPartial: string; // Original typed partial
  };
  let tabCompletionState = $state<TabCompletionState | null>(null);

  // Emoji autocomplete state
  type EmojiAutocompleteState = {
    query: string; // Search query (without leading colon)
    triggerStart: number; // Position of : in text (relative to full text before cursor)
  };
  let emojiAutocomplete = $state<EmojiAutocompleteState | null>(null);
  let emojiAutocompleteRef = $state<{ handleKeyDown: (e: KeyboardEvent) => boolean } | null>(null);

  // Mention autocomplete state
  type MentionAutocompleteState = {
    query: string; // Search query (without leading @)
    triggerStart: number; // Position of @ in text
  };
  let mentionAutocomplete = $state<MentionAutocompleteState | null>(null);
  let mentionAutocompleteRef = $state<{ handleKeyDown: (e: KeyboardEvent) => boolean } | null>(
    null
  );

  // Close autocomplete popups when switching rooms
  $effect(() => {
    void roomId;
    emojiAutocomplete = null;
    mentionAutocomplete = null;
  });

  // Detect :emoji pattern at cursor (requires at least 2 chars after :)
  function getEmojiPartialAtCursor(): { query: string; start: number } | null {
    if (!editorApi) return null;

    const textBefore = editorApi.getTextBeforeCursor();

    // Match :word at end of text (requires at least 2 chars after :)
    // Must be preceded by whitespace, start of string, or another emoji
    const match = textBefore.match(/(?:^|[\s]):([\w]{2,})$/);
    if (!match) return null;

    return {
      query: match[1],
      start: textBefore.length - match[1].length - 1 // Position of :
    };
  }

  // Update emoji autocomplete state when input changes
  function updateEmojiAutocomplete() {
    const partial = getEmojiPartialAtCursor();
    if (partial && searchEmojis(partial.query, 1).length > 0) {
      emojiAutocomplete = {
        query: partial.query,
        triggerStart: partial.start
      };
      // Only one popup at a time
      mentionAutocomplete = null;
    } else {
      emojiAutocomplete = null;
    }
  }

  // Handle emoji selection from autocomplete
  function handleEmojiSelect(emoji: string, _name: string) {
    if (!emojiAutocomplete || !editorApi) return;

    const textBefore = editorApi.getTextBeforeCursor();
    const charsToReplace = textBefore.length - emojiAutocomplete.triggerStart;
    editorApi.replaceTextBeforeCursor(charsToReplace, emoji + ' ');
    emojiAutocomplete = null;
  }

  function closeEmojiAutocomplete() {
    emojiAutocomplete = null;
  }

  // Update mention autocomplete state when input changes
  function updateMentionAutocomplete() {
    // Don't show mention popup if emoji popup is active
    if (emojiAutocomplete) {
      mentionAutocomplete = null;
      return;
    }

    const partial = getMentionPartialAtCursor();
    if (partial && findMatchingMembers(partial.partial).length > 0) {
      mentionAutocomplete = {
        query: partial.partial,
        triggerStart: partial.start
      };
    } else {
      mentionAutocomplete = null;
    }
  }

  // Handle mention selection from autocomplete
  function handleMentionSelect(login: string, viaTab: boolean) {
    if (!mentionAutocomplete) return;

    const triggerStart = mentionAutocomplete.triggerStart;
    const originalPartial = mentionAutocomplete.query;

    applyCompletion(login, triggerStart);
    mentionAutocomplete = null;

    // When selected via Tab, set up tab completion state so subsequent
    // Tab presses cycle through the other candidates (matching existing behavior).
    if (viaTab) {
      const candidates = findMatchingMembers(originalPartial);
      if (candidates.length > 1) {
        const selectedIdx = candidates.indexOf(login);
        tabCompletionState = {
          candidates,
          index: selectedIdx >= 0 ? selectedIdx : 0,
          triggerStart,
          originalPartial
        };
      }
    }
  }

  function closeMentionAutocomplete() {
    mentionAutocomplete = null;
  }

  // Find usernames that match a partial string using fuzzy matching
  function findMatchingMembers(partial: string): string[] {
    const scored: { login: string; score: number }[] = [];

    for (const m of members) {
      const loginScore = fuzzyMatch(partial, m.login);
      const displayScore = fuzzyMatch(partial, m.displayName);
      const bestScore = Math.max(loginScore ?? -1, displayScore ?? -1);

      if (bestScore > 0) {
        scored.push({ login: m.login, score: bestScore });
      }
    }

    // Sort by score (descending) so better matches come first
    scored.sort((a, b) => b.score - a.score);

    return scored.map((s) => s.login);
  }

  // Detect if cursor is at end of @partial pattern (at least 1 char after @)
  function getMentionPartialAtCursor(): { partial: string; start: number } | null {
    if (!editorApi) return null;

    const textBefore = editorApi.getTextBeforeCursor();

    // Match @word at end of text (requires at least 1 char after @)
    // Must be preceded by whitespace or start of string
    // Includes dots for usernames like "hendrik.mans"
    const match = textBefore.match(/(?:^|[\s])@([a-zA-Z0-9_.-]+)$/);
    if (!match) return null;

    return {
      partial: match[1],
      start: textBefore.length - match[1].length - 1 // Position of @
    };
  }

  // Apply a completion - replace @partial with @username + space
  function applyCompletion(username: string, atPosition: number) {
    if (!editorApi) return;

    const textBefore = editorApi.getTextBeforeCursor();
    const charsToReplace = textBefore.length - atPosition;
    editorApi.replaceTextBeforeCursor(charsToReplace, '@' + username + ' ');
  }

  // Handle Tab key for @mention autocomplete
  function handleTabCompletion(event: KeyboardEvent): boolean {
    // First, check if we're continuing an active completion cycle
    // (cursor is right after a completed username + space)
    if (tabCompletionState && tabCompletionState.candidates.length > 1) {
      const currentUsername = tabCompletionState.candidates[tabCompletionState.index];
      const expectedCursorPos = tabCompletionState.triggerStart + 1 + currentUsername.length + 1; // @ + username + space
      const currentPos = editorApi?.getTextBeforeCursor().length ?? 0;

      if (currentPos === expectedCursorPos) {
        // Continue cycling through candidates
        event.preventDefault();
        const nextIndex = (tabCompletionState.index + 1) % tabCompletionState.candidates.length;
        tabCompletionState = { ...tabCompletionState, index: nextIndex };
        applyCompletion(tabCompletionState.candidates[nextIndex], tabCompletionState.triggerStart);
        return true;
      }
    }

    // Check for new @mention partial
    const mentionInfo = getMentionPartialAtCursor();
    if (!mentionInfo || mentionInfo.partial.length === 0) {
      return false; // Let default Tab behavior happen
    }

    event.preventDefault();

    // Start new completion
    const candidates = findMatchingMembers(mentionInfo.partial);
    if (candidates.length > 0) {
      tabCompletionState = {
        candidates,
        index: 0,
        triggerStart: mentionInfo.start,
        originalPartial: mentionInfo.partial
      };
      applyCompletion(candidates[0], mentionInfo.start);
    }

    return true;
  }

  function formatFileSize(bytes: number): string {
    if (bytes >= 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(0)} MB`;
    if (bytes >= 1024) return `${(bytes / 1024).toFixed(0)} KB`;
    return `${bytes} bytes`;
  }

  /**
   * Validate file sizes against instance limits.
   * Returns only the files that pass validation, toasting errors for rejected ones.
   */
  function validateFileSizes(files: File[]): File[] {
    const accepted: File[] = [];
    for (const file of files) {
      const isVideo = file.type.startsWith('video/');
      const limit = isVideo ? instanceState.maxVideoUploadSize : instanceState.maxUploadSize;
      if (file.size > limit) {
        toast.error(
          `${file.name} is too large (${formatFileSize(file.size)}). Maximum is ${formatFileSize(limit)}.`
        );
      } else {
        accepted.push(file);
      }
    }
    return accepted;
  }

  async function handleFileSelect(event: Event) {
    const target = event.target as HTMLInputElement;
    if (target.files) {
      const validFiles = validateFileSizes(Array.from(target.files));
      const prepared = await prepareFiles(validFiles);
      const newFiles = prepared.map((file) => ({
        file,
        url: URL.createObjectURL(file)
      }));
      filesWithUrls = [...filesWithUrls, ...newFiles];
    }
    // Reset input so same file can be selected again
    target.value = '';
  }

  function removeFile(index: number) {
    const removed = filesWithUrls[index];
    if (removed) {
      URL.revokeObjectURL(removed.url);
    }
    filesWithUrls = filesWithUrls.filter((_, i) => i !== index);
  }

  /**
   * Add files from an external source (e.g., drag-and-drop).
   * Creates object URLs for preview and adds to the attachment list.
   */
  async function addFiles(files: File[]) {
    const validFiles = validateFileSizes(files);
    const prepared = await prepareFiles(validFiles);
    const newFiles = prepared.map((file) => ({
      file,
      url: URL.createObjectURL(file)
    }));
    filesWithUrls = [...filesWithUrls, ...newFiles];
  }

  // Focus the input programmatically (e.g., when opening thread from mobile action sheet)
  function focus() {
    tick().then(() => editorApi?.focus());
  }

  // Expose API to parent via onReady callback
  $effect(() => {
    onReady?.({ addFiles, focus });
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
      const validFiles = validateFileSizes(pastedFiles);
      // Fire-and-forget: convert HEIC files asynchronously, then add to list
      prepareFiles(validFiles).then((prepared) => {
        const newFiles = prepared.map((file) => ({ file, url: URL.createObjectURL(file) }));
        filesWithUrls = [...filesWithUrls, ...newFiles];
      });
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

  async function postMessage() {
    // Require either non-empty message body or attachments.
    // hasVisibleContent rejects messages with only invisible Unicode characters.
    const bodyToSend = normalizeMessageBody(message.trim());
    const hasBody = hasVisibleContent(bodyToSend);
    const filesToSend = selectedFiles.length > 0 ? [...selectedFiles] : null;
    if (!hasBody && !filesToSend) return;

    // Capture active link preview before clearing state
    const previewURL = detectedURLs[0];
    const activePreview = previewURL ? previews.get(previewURL) : null;
    const previewFields = activePreview ? useFragment(LinkPreviewFragment, activePreview) : null;
    const linkPreviewInput =
      activePreview && previewFields && !dismissedURLs.has(previewURL)
        ? {
            url: previewFields.url,
            title: previewFields.title,
            description: previewFields.description,
            siteName: previewFields.siteName,
            imageAssetId: activePreview.imageAssetId,
            embedType: previewFields.embedType,
            embedId: previewFields.embedId
          }
        : null;

    // Optimistically clear the editor so the user can start typing the next
    // message immediately (matches Slack/Discord behavior).
    message = '';
    editorApi?.setContent('');
    for (const { url } of filesWithUrls) {
      URL.revokeObjectURL(url);
    }
    filesWithUrls = [];
    clearLinkPreviews();

    loading = true;

    try {
      const response = await connection().client.mutation(PostMessageMutation, {
        input: {
          spaceId,
          roomId,
          body: bodyToSend || null,
          attachments: filesToSend,
          inThread: inThread ?? null,
          inReplyTo: inReplyTo ?? null,
          linkPreview: linkPreviewInput,
          alsoSendToChannel: alsoSendToChannel || null
        }
      });

      if (response.error) {
        toast.error('Failed to send message');
        console.error('Error posting message:', response.error);
        // Restore message so user can retry without retyping
        message = bodyToSend;
        editorApi?.setContent(bodyToSend);
        if (filesToSend) {
          filesWithUrls = filesToSend.map((f) => ({ file: f, url: URL.createObjectURL(f) }));
        }
      } else {
        // Request scroll to bottom so user sees their new message.
        // Only for main room posts — thread replies should not disturb
        // the main chat's scroll position. The thread EventList handles
        // its own scrolling via updateCounter.
        if (!inThread) {
          scrollState?.requestScrollToBottom();
        }

        // Clear reply-in-room state after sending
        onCancelReply?.();

        // Mark this room as read (we just posted, so we've seen all messages)
        roomUnreadStore.setRoomUnread(spaceId, roomId, false);

        // Reset "also send to channel" checkbox after successful send
        alsoSendToChannel = false;

        // Notify parent that message was sent (for resetting typing indicator debounce)
        onMessageSent?.();
      }
    } finally {
      loading = false;
    }
  }

  async function editMessage() {
    const trimmedBody = normalizeMessageBody(message.trim());
    if (!trimmedBody) {
      toast.error('Message cannot be empty');
      return;
    }

    const eventId = editState.eventId;
    if (!eventId) return;

    loading = true;

    const response = await connection().client.mutation(EditMessageMutation, {
      input: { spaceId, roomId, eventId, body: trimmedBody }
    });

    if (response.error) {
      toast.error(response.error.message || 'Failed to edit message');
    } else {
      message = '';
      editorApi?.setContent('');
      editState.cancelEdit();
    }

    loading = false;
  }

  async function handleSubmit() {
    if (loading) return; // Guard against double-sends while editor stays editable
    if (isEditing) {
      await editMessage();
    } else {
      await postMessage();
    }
  }

  function cancelEdit() {
    editState.cancelEdit();
    message = '';
    editorApi?.setContent('');
  }

  // Handle keyboard events from TipTap editor.
  // Return true to prevent TipTap's default handling.
  function handleEditorKeyDown(event: KeyboardEvent): boolean {
    // Handle emoji autocomplete keyboard events first
    if (emojiAutocomplete && emojiAutocompleteRef) {
      if (emojiAutocompleteRef.handleKeyDown(event)) {
        return true;
      }
    }

    // Handle mention autocomplete keyboard events
    if (mentionAutocomplete && mentionAutocompleteRef) {
      if (mentionAutocompleteRef.handleKeyDown(event)) {
        return true;
      }
    }

    // Handle Tab for @mention autocomplete
    if (event.key === 'Tab') {
      if (handleTabCompletion(event)) {
        return true;
      }
      // If no completion happened, let default Tab behavior occur
    }

    // Reset tab-completion state on any other key
    if (event.key !== 'Tab') {
      tabCompletionState = null;
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
        editState.startEdit(lastMsg.eventId, lastMsg.body);
        return true;
      }
    }

    if (event.key === 'Enter' && !event.shiftKey && !isTouchDevice()) {
      handleSubmit(); // Fire-and-forget (async, but keydown must return sync)
      return true;
    }

    return false; // Let TipTap handle it (e.g., Shift+Enter for hard break)
  }

  // Handle content updates from TipTap editor
  function handleEditorUpdate(text: string) {
    const previousMessage = message;
    message = text;
    // Only trigger typing indicator for actual user input. Programmatic
    // setContent calls (drafts, edit mode) always set `message` before
    // calling setContent, so by the time TipTap fires onUpdate, message
    // already equals the new text — this guard skips those updates.
    if (text !== previousMessage) {
      onTyping?.();
    }
    updateEmojiAutocomplete();
    updateMentionAutocomplete();
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
  bind:this={composerEl}
  class="mt-2 flex flex-col gap-2 p-2"
  onclick={(e) => {
    if (!(e.target as HTMLElement).closest('button, a, input, label, .tiptap')) {
      editorApi?.focus();
    }
  }}
>
  <!-- Link / message preview -->
  {#if detectedURLs[0]}
    {@const url = detectedURLs[0]}
    {@const messageLink = parseMessageLink(url)}
    {#if messageLink}
      <MessagePreviewCard link={messageLink} onDismiss={() => dismissPreview(url)} />
    {:else if fetchingURLs.has(url)}
      <LinkPreviewSkeleton />
    {:else if previews.get(url)}
      <LinkPreviewCard preview={previews.get(url)!} onDismiss={() => dismissPreview(url)} />
    {/if}
  {/if}

  <!-- Selected files preview -->
  {#if filesWithUrls.length > 0}
    <div class="flex flex-wrap gap-2 rounded-lg bg-surface-300 p-2">
      {#each filesWithUrls as { file, url }, index (index)}
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
              <span class="iconify uil--music text-lg text-muted"></span>
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
  <input
    bind:this={fileInputElement}
    type="file"
    accept="image/*,video/*,audio/*"
    multiple
    onchange={handleFileSelect}
    class="hidden"
  />

  <!-- Unified input container -->
  <div
    class="relative flex items-start gap-4 rounded-lg bg-surface py-2 pr-3"
    class:pl-3={isEditing}
    class:opacity-50={inputDisabled}
    class:sending={loading}
  >
    <!-- Emoji autocomplete popup -->
    {#if emojiAutocomplete}
      <EmojiAutocomplete
        bind:this={emojiAutocompleteRef}
        query={emojiAutocomplete.query}
        onSelect={handleEmojiSelect}
        onClose={closeEmojiAutocomplete}
      />
    {/if}

    <!-- Mention autocomplete popup -->
    {#if mentionAutocomplete}
      <MentionAutocomplete
        bind:this={mentionAutocompleteRef}
        query={mentionAutocomplete.query}
        {members}
        onSelect={handleMentionSelect}
        onClose={closeMentionAutocomplete}
      />
    {/if}
    <!-- Attachment button - hidden in edit mode (editMessage only supports text) -->
    {#if !isEditing}
      <button
        type="button"
        onclick={() => fileInputElement?.click()}
        disabled={inputDisabled}
        class="flex h-8 w-12 shrink-0 cursor-pointer items-center justify-center rounded text-muted transition-colors duration-100 enabled:hover:text-text disabled:cursor-not-allowed"
        title="Attach file"
      >
        <span class="iconify text-xl uil--image-upload"></span>
      </button>
    {/if}

    <!-- Text input (TipTap editor) -->
    <TipTapEditor
      placeholder={currentPlaceholder}
      editable={!inputDisabled}
      autofocus={autoFocus && shouldAutoFocus()}
      {testid}
      onUpdate={handleEditorUpdate}
      onKeyDown={handleEditorKeyDown}
      onPaste={handlePaste}
      onReady={handleEditorReady}
    />

    <!-- Send button -->
    <button
      type="button"
      onpointerdown={(e) => e.preventDefault()}
      onclick={handleSubmit}
      disabled={!canSubmit}
      class="flex h-8 w-8 shrink-0 cursor-pointer items-center justify-center rounded text-muted transition-colors duration-100 enabled:hover:text-text disabled:cursor-not-allowed disabled:opacity-50"
      title="Send message"
    >
      <span class="iconify text-xl uil--telegram-alt"></span>
    </button>
  </div>

  <!-- Also send to channel checkbox (thread replies only, when permitted) -->
  {#if showAlsoSendToChannel && !isEditing}
    <label class="flex cursor-pointer items-center gap-2 px-3 text-sm text-muted">
      <input
        type="checkbox"
        bind:checked={alsoSendToChannel}
        disabled={inputDisabled}
        class="cursor-pointer accent-primary"
      />
      Also send to channel
    </label>
  {/if}

  <!-- Reply indicator -->
  {#if inReplyTo && replyDisplayName}
    <div
      data-testid="reply-indicator"
      class="flex items-center justify-between rounded-md bg-surface-200 px-3 py-2 text-sm"
    >
      <span class="min-w-0 truncate text-text">
        Replying to <strong>{replyDisplayName}</strong>
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
        <kbd class="rounded bg-surface-300 px-1.5 py-0.5 text-xs">Esc</kbd> to cancel
      </button>
      <!-- Mobile: visible "Cancel" button -->
      <button
        type="button"
        onclick={() => onCancelReply?.()}
        class="shrink-0 cursor-pointer rounded bg-surface-300 px-2.5 py-1 text-xs font-medium text-text transition-colors hover:bg-surface-highlighted sm:hidden"
      >
        Cancel
      </button>
    </div>
  {/if}

  <!-- Edit mode indicator -->
  {#if isEditing}
    <div class="flex items-center justify-between rounded-md bg-surface-200 px-3 py-2 text-sm">
      <span class="text-text">Editing message</span>
      <!-- Desktop: clickable "Esc to cancel" -->
      <button
        type="button"
        onclick={cancelEdit}
        class="hidden cursor-pointer items-center gap-1 text-muted transition-colors hover:text-text sm:flex"
      >
        <kbd class="rounded bg-surface-300 px-1.5 py-0.5 text-xs">Esc</kbd> to cancel
      </button>
      <!-- Mobile: visible "Cancel" button -->
      <button
        type="button"
        onclick={cancelEdit}
        class="cursor-pointer rounded bg-surface-300 px-2.5 py-1 text-xs font-medium text-text transition-colors hover:bg-surface-highlighted sm:hidden"
      >
        Cancel
      </button>
    </div>
  {/if}
</div>

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
