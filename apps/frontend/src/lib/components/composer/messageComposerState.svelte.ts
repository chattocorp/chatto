import { tick, untrack } from 'svelte';
import type { TimelineEventView } from '$lib/render/timelineEvents';
import type {
  ComposerContext,
  QuoteInsertionContent,
  RoomMember,
  RoomMembersStore
} from '$lib/state/room';
import type { MentionRolesStore } from '$lib/state/server/mentionRoles.svelte';
import type { RoomUnreadStore } from '$lib/state/server/roomUnread.svelte';
import type { ServerInfoState } from '$lib/state/server/state.svelte';
import type { createMessageAPI, UpdateMessageInput } from '$lib/api-client/messages';
import type { createLinkPreviewAPI } from '$lib/api-client/linkPreviews';
import { hasVisibleContent } from '$lib/validation';
import { shouldAutoFocus } from '$lib/utils/shouldAutoFocus';
import { prefersTouchActions } from '$lib/utils/inputCapabilities';
import type { ComposerSendMode } from '$lib/state/userPreferences.svelte';
import { useDebounce } from '$lib/hooks/useDebounce.svelte';
import { toast } from '$lib/ui/toast';
import { m } from '$lib/i18n/messages';
import { AttachmentsState } from './attachments.svelte';
import { AutocompleteState, type MentionRole } from './autocomplete.svelte';
import { DraftState, draftKey } from './draft.svelte';
import {
  emptyComposerIndentState,
  type ComposerEditorApi,
  type ComposerFormattingState,
  type ComposerIndentState
} from './editorTypes';
import { LinkPreviewState } from './linkPreviews.svelte';
import { ComposerSubmissionState, type PreparedPost } from './submission.svelte';

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

export type MessageComposerApi = {
  addFiles: (files: File[]) => void;
  focus: () => void;
  insertQuote: (text: QuoteInsertionContent) => void;
};

export type RecentThreadRootCandidate = {
  threadRootEventId: string;
};

export type MessageComposerProps = {
  roomId: string;
  inThread?: string;
  inReplyTo?: string;
  replyDisplayName?: string;
  replyExcerpt?: string;
  placeholder?: string;
  canPost?: boolean;
  canAttach?: boolean;
  slowModeSeconds?: number;
  slowModeNextPostAt?: string | null;
  slowModeBypassed?: boolean;
  autoFocus?: boolean;
  onReady?: (api: MessageComposerApi) => void;
  onTyping?: () => void;
  onMessageSent?: (event: TimelineEventView | null) => void;
  /** Called after a room-level post successfully creates a thread. */
  onThreadCreated?: (threadRootEventId: string) => void;
  onCancelReply?: () => void;
  onEscape?: () => void;
  showAlsoSendToChannel?: boolean;
  showCreateThread?: boolean;
  createThreadRequired?: boolean;
  createThreadDefault?: boolean;
  threadsEncouraged?: boolean;
  getRecentThreadRootCandidate?: () => RecentThreadRootCandidate | null;
  onThreadMessageSent?: (
    threadRootEventId: string,
    event: TimelineEventView | null
  ) => void;
};

type MessageComposerDependencies = {
  getRoomId: () => string;
  getThreadRootEventId: () => string | undefined;
  getReplyEventId: () => string | undefined;
  getCanPost: () => boolean;
  getCanAttach: () => boolean;
  getSlowModeBlocked: () => boolean;
  getCanCreateThread: () => boolean;
  getCreateThreadRequired: () => boolean;
  getCreateThreadDefault: () => boolean;
  getRecentThreadRootCandidate: () => RecentThreadRootCandidate | null;
  getAutoFocus: () => boolean;
  getComposerSendMode: () => ComposerSendMode;
  getPlaceholder: () => string | undefined;
  getOnReady: () => MessageComposerProps['onReady'];
  getCallbacks: () => Pick<
    MessageComposerProps,
    | 'onTyping'
    | 'onMessageSent'
    | 'onThreadCreated'
    | 'onThreadMessageSent'
    | 'onCancelReply'
    | 'onEscape'
  >;
  onPostError?: (error: unknown) => boolean;
  context: ComposerContext;
  getMembers: () => RoomMember[];
  membersStore: RoomMembersStore;
  mentionRolesStore: MentionRolesStore;
  serverInfo: ServerInfoState;
  roomUnreadStore: RoomUnreadStore;
  getMessageAPI: () => ReturnType<typeof createMessageAPI>;
  getLinkPreviewAPI: () => ReturnType<typeof createLinkPreviewAPI>;
  isConnectionLost: () => boolean;
};

export function bodyForSend(text: string): string {
  const normalized = text.replace(/\n{3,}/g, '\n\n');
  const hasStructuralBody = normalized
    .split('\n')
    .some((line) => /^ {0,3}(?:#{1,6}|[-+*]|\d{1,9}[.)]|>)[ \t]$/.test(line));
  return hasStructuralBody ? normalized : text.trim().replace(/\n{3,}/g, '\n\n');
}

/**
 * Owns the state transitions for one mounted room or thread composer.
 *
 * The Svelte component remains the DOM and context boundary. This class keeps
 * draft, edit, autocomplete, attachment, preview, and submission transitions
 * together so every reset follows the same path.
 */
export class MessageComposerState {
  message = $state('');
  editorApi = $state.raw<ComposerEditorApi | null>(null);
  fileInputElement = $state<HTMLInputElement>();
  formattingState = $state<ComposerFormattingState>({ ...emptyFormattingState });
  indentState = $state<ComposerIndentState>({ ...emptyComposerIndentState });
  alsoSendToChannel = $state(false);
  createThread = $state(false);
  mentionSearchMembers = $state.raw<RoomMember[]>([]);

  readonly draft = new DraftState();
  readonly attachments: AttachmentsState;
  readonly linkPreviews: LinkPreviewState;
  readonly autocomplete: AutocompleteState;
  readonly submission: ComposerSubmissionState;

  readonly #dependencies: MessageComposerDependencies;
  readonly #mentionSearchDebounce = useDebounce();
  #mentionSearchRequestId = 0;
  #editSeededForEvent = '';
  #autocompleteRoomId = '';
  #insertedQuoteRequestId = 0;
  #focusRequested = false;
  #threadCreationPolicyKey = '';
  #threadDestinationChoicePending = false;
  pendingThreadDestinationConfirmation = $state<{
    post: PreparedPost;
    candidate: RecentThreadRootCandidate;
  } | null>(null);

  constructor(dependencies: MessageComposerDependencies) {
    this.#dependencies = dependencies;
    this.attachments = new AttachmentsState(() => dependencies.serverInfo);
    this.linkPreviews = new LinkPreviewState(dependencies.getLinkPreviewAPI);
    this.autocomplete = new AutocompleteState(
      () => this.editorApi,
      () => this.mentionCandidates,
      () => this.mentionRoles
    );
    this.submission = new ComposerSubmissionState({
      getAPI: dependencies.getMessageAPI,
      getMentionRoleStatus: () => dependencies.mentionRolesStore.status,
      loadMentionRoles: () => dependencies.mentionRolesStore.load(),
      getMentionRoleNames: () => this.mentionRoles.map((role) => role.name),
      onPostSuccess: (post, event) => this.#handlePostSuccess(post, event),
      onPostError: dependencies.onPostError,
      onEditSuccess: () => this.#handleEditSuccess()
    });

    void dependencies.mentionRolesStore.load();
    this.#synchronizeMentionSearch();
    this.#synchronizeEditState();
    this.#synchronizeDraft();
    this.#synchronizeDraftText();
    this.#synchronizeLinkPreviews();
    this.#synchronizeAttachmentPermission();
    this.#synchronizeThreadCreationPolicy();
    this.#synchronizeAutoFocus();
    this.#synchronizeQuoteInsertion();
    this.#synchronizePublicApi();
  }

  get editState() {
    return this.#dependencies.context.editState;
  }

  get isEditing(): boolean {
    return this.editState.eventId !== null;
  }

  get mentionRoles(): MentionRole[] {
    return this.#dependencies.mentionRolesStore.roles;
  }

  get mentionCandidates(): RoomMember[] {
    return this.mentionSearchMembers.length > 0
      ? this.mentionSearchMembers
      : this.#dependencies.getMembers();
  }

  get draftKey(): string {
    return draftKey(this.#dependencies.getRoomId(), this.#dependencies.getThreadRootEventId());
  }

  get currentPlaceholder(): string {
    return this.isEditing
      ? m('composer.editing_placeholder')
      : (this.#dependencies.getPlaceholder() ?? m('composer.placeholder'));
  }

  get testid(): string {
    return this.#dependencies.getThreadRootEventId() ? 'thread-reply-input' : 'message-input';
  }

  get showEditEchoToggle(): boolean {
    return (
      this.isEditing &&
      this.editState.threadRootEventId !== null &&
      (this.editState.channelEchoEventId !== null || this.editState.canAddChannelEcho)
    );
  }

  get inputDisabled(): boolean {
    return (
      this.submission.loading ||
      (!this.#dependencies.getCanPost() && !this.isEditing) ||
      this.#dependencies.isConnectionLost()
    );
  }

  get hasSendableAttachments(): boolean {
    return this.#dependencies.getCanAttach() && this.attachments.selectedFiles.length > 0;
  }

  get canSubmit(): boolean {
    return (
      !this.submission.loading &&
      !this.submission.roleMentionCheckLoading &&
      !this.inputDisabled &&
      (this.isEditing || !this.#dependencies.getSlowModeBlocked()) &&
      this.attachments.pendingCount === 0 &&
      (hasVisibleContent(this.message) || this.hasSendableAttachments || this.isEditing)
    );
  }

  observeResize = (node: HTMLDivElement) => {
    const scrollState = this.#dependencies.context.scrollState;
    if (!scrollState) return;
    const observer = new ResizeObserver(() => scrollState.scrollToBottomIfSticky());
    observer.observe(node);
    return () => observer.disconnect();
  };

  handleFileSelect(event: Event): void {
    const input = event.target as HTMLInputElement;
    if (!this.#dependencies.getCanAttach() || this.inputDisabled) {
      input.value = '';
      return;
    }
    if (input.files) void this.attachments.stageFiles(Array.from(input.files));
    input.value = '';
  }

  async addFiles(files: File[]): Promise<void> {
    if (!this.#dependencies.getCanAttach() || this.inputDisabled) return;
    await this.attachments.stageFiles(files);
  }

  focus(): void {
    const api = this.editorApi;
    if (!api) {
      this.#focusRequested = true;
      return;
    }

    this.#focusRequested = false;
    tick().then(() => {
      if (this.editorApi === api) api.focus();
    });
  }

  insertQuote(text: QuoteInsertionContent): void {
    tick().then(() => this.editorApi?.insertQuote(text));
  }

  handlePaste(event: ClipboardEvent): boolean {
    if (this.isEditing || this.inputDisabled) return false;
    const items = event.clipboardData?.items;
    if (!items) return false;

    const files = Array.from(items)
      .filter((item) => item.type.startsWith('image/'))
      .map((item) => item.getAsFile())
      .filter((file): file is File => file !== null);
    if (files.length === 0) return false;
    if (this.#dependencies.getCanAttach()) void this.attachments.stageFiles(files);
    return true;
  }

  async submit(): Promise<void> {
    if (
      this.submission.loading ||
      this.submission.roleMentionCheckLoading ||
      this.submission.roleMentionConfirmationLoading ||
      this.submission.pendingRoleMentionConfirmation ||
      this.pendingThreadDestinationConfirmation ||
      this.inputDisabled ||
      (!this.isEditing && this.#dependencies.getSlowModeBlocked()) ||
      this.attachments.pendingCount > 0
    ) {
      return;
    }
    await (this.isEditing ? this.#editMessage() : this.#createMessage());
  }

  cancelThreadDestinationConfirmation(): void {
    if (this.#threadDestinationChoicePending) return;
    this.pendingThreadDestinationConfirmation = null;
  }

  async postInRecentThread(): Promise<void> {
    const pending = this.pendingThreadDestinationConfirmation;
    if (!pending || this.#threadDestinationChoicePending) return;
    this.#threadDestinationChoicePending = true;
    this.pendingThreadDestinationConfirmation = null;
    try {
      await this.submission.requestPost({
        ...pending.post,
        threadRootEventId: pending.candidate.threadRootEventId,
        createThread: false
      });
    } finally {
      this.#threadDestinationChoicePending = false;
    }
  }

  async postAsNewRoot(): Promise<void> {
    const pending = this.pendingThreadDestinationConfirmation;
    if (!pending || this.#threadDestinationChoicePending) return;
    this.#threadDestinationChoicePending = true;
    this.pendingThreadDestinationConfirmation = null;
    try {
      await this.submission.requestPost(pending.post);
    } finally {
      this.#threadDestinationChoicePending = false;
    }
  }

  cancelEdit(): void {
    this.#resetEditor();
    this.editState.cancelEdit();
  }

  handleEditorKeyDown(event: KeyboardEvent): boolean {
    if (this.autocomplete.emoji?.query && this.autocomplete.emojiRef?.handleKeyDown(event)) {
      return true;
    }
    if (this.autocomplete.mention?.query && this.autocomplete.mentionRef?.handleKeyDown(event)) {
      return true;
    }
    if (event.key === 'Enter' && this.#handleEnter(event)) return true;
    if (event.key === 'Tab' && this.autocomplete.handleTabCompletion(event)) return true;
    if (event.key !== 'Tab') this.autocomplete.resetTabCompletion();
    if (event.key === 'Escape' && this.#handleEscape()) return true;
    if (event.key === 'ArrowUp' && this.#editLastMessage()) return true;
    return false;
  }

  handleEditorUpdate(text: string): void {
    const changed = text !== this.message;
    this.message = text;
    if (changed) this.#dependencies.getCallbacks().onTyping?.();
    this.autocomplete.update();
  }

  handleEditorReady(api: ComposerEditorApi): void {
    this.editorApi = api;
    if (this.message) api.setContent(this.message);
    if (this.#focusRequested) this.focus();
  }

  handleEditorDestroyed(api: ComposerEditorApi): void {
    if (this.editorApi === api) this.editorApi = null;
  }

  #synchronizeMentionSearch(): void {
    $effect(() => {
      const query = this.autocomplete.mention?.query ?? null;
      const requestId = ++this.#mentionSearchRequestId;
      this.#mentionSearchDebounce.cancel();
      if (!query) {
        this.mentionSearchMembers = [];
        return;
      }
      this.#mentionSearchDebounce.run(() => {
        void this.#dependencies.membersStore.searchMembers(query).then((results) => {
          if (requestId === this.#mentionSearchRequestId) this.mentionSearchMembers = results;
        });
      }, 150);
    });
  }

  #synchronizeEditState(): void {
    $effect(() => {
      const eventId = this.editState.eventId;
      const originalBody = this.editState.originalBody;
      const api = this.editorApi;
      if (eventId && originalBody && this.#editSeededForEvent !== eventId) {
        this.#editSeededForEvent = eventId;
        this.autocomplete.reset();
        this.draft.clearText();
        this.message = originalBody;
        this.alsoSendToChannel = this.editState.channelEchoEventId !== null;
        api?.setContent(originalBody);
        tick().then(() => api?.focus('end'));
        this.attachments.clear();
        this.linkPreviews.clear();
      } else if (this.#editSeededForEvent && !eventId) {
        this.#resetEditor();
        this.#editSeededForEvent = '';
      }
    });
  }

  #synchronizeDraft(): void {
    $effect(() => {
      const roomId = this.#dependencies.getRoomId();
      if (this.#autocompleteRoomId !== roomId) {
        this.#autocompleteRoomId = roomId;
        this.autocomplete.resetForRoom();
      }
      if (this.isEditing) {
        this.draft.switchKey(this.draftKey);
        this.attachments.restore([]);
        return;
      }
      const draftMessage = this.draft.switchKey(this.draftKey);
      this.message = draftMessage;
      this.editorApi?.setContent(draftMessage);
      this.attachments.restore(untrack(() => this.draft.takeFiles()));
      return () => this.draft.stashFiles(untrack(() => this.attachments.filesWithUrls));
    });
  }

  #synchronizeDraftText(): void {
    $effect(() => {
      void this.draftKey;
      if (!this.isEditing) this.draft.persistText(this.message);
    });
  }

  #synchronizeLinkPreviews(): void {
    $effect(() => this.linkPreviews.scheduleDetection(this.message, this.isEditing));
  }

  #synchronizeAttachmentPermission(): void {
    $effect(() => {
      if (!this.#dependencies.getCanAttach() && this.attachments.filesWithUrls.length > 0) {
        this.attachments.clear();
      }
    });
  }

  #synchronizeThreadCreationPolicy(): void {
    $effect(() => {
      const roomId = this.#dependencies.getRoomId();
      const canCreateThread = this.#dependencies.getCanCreateThread();
      const required = this.#dependencies.getCreateThreadRequired();
      const defaultEnabled = this.#dependencies.getCreateThreadDefault();
      const policyKey = `${roomId}\u0000${canCreateThread}\u0000${required}\u0000${defaultEnabled}`;
      if (policyKey === this.#threadCreationPolicyKey) return;
      this.#threadCreationPolicyKey = policyKey;
      this.createThread = canCreateThread && !required && defaultEnabled;
    });
  }

  #synchronizeAutoFocus(): void {
    $effect(() => {
      const autoFocus = this.#dependencies.getAutoFocus();
      const roomId = this.#dependencies.getRoomId();
      const inReplyTo = this.#dependencies.getReplyEventId();
      void roomId;
      void inReplyTo;
      if (autoFocus && shouldAutoFocus() && this.editorApi && !this.inputDisabled) {
        tick().then(() => this.editorApi?.focus());
      }
    });
  }

  #synchronizeQuoteInsertion(): void {
    $effect(() => {
      const request = this.#dependencies.context.quoteInsertionState.request;
      const api = this.editorApi;
      if (!request || !api || request.id === this.#insertedQuoteRequestId) return;
      this.#insertedQuoteRequestId = request.id;
      api.insertQuote(request.text);
    });
  }

  #synchronizePublicApi(): void {
    $effect(() => {
      const onReady = this.#dependencies.getOnReady();
      untrack(() => {
        onReady?.({
          addFiles: (files) => void this.addFiles(files),
          focus: () => this.focus(),
          insertQuote: (text) => this.insertQuote(text)
        });
      });
    });
  }

  #resetEditor(): void {
    this.autocomplete.reset();
    this.message = '';
    this.alsoSendToChannel = false;
    this.createThread =
      this.#dependencies.getCanCreateThread() &&
      !this.#dependencies.getCreateThreadRequired() &&
      this.#dependencies.getCreateThreadDefault();
    this.editorApi?.setContent('');
  }

  #handlePostSuccess(post: PreparedPost, event: TimelineEventView | null): void {
    const activeDraftWasSent = this.draftKey === post.draftKey;
    const stashedFiles = this.draft.discardFiles(post.draftKey);
    this.draft.clearText(post.draftKey);
    if (activeDraftWasSent) {
      this.#resetEditor();
      this.attachments.clear();
      this.linkPreviews.clear();
      const callbacks = this.#dependencies.getCallbacks();
      if (post.threadRootEventId && callbacks.onThreadMessageSent) {
        callbacks.onThreadMessageSent(post.threadRootEventId, event);
      } else {
        callbacks.onMessageSent?.(event);
        if (post.createThread && event) {
          callbacks.onThreadCreated?.(event.id);
        }
      }
      this.#dependencies.context.scrollState?.requestScrollToBottom();
      callbacks.onCancelReply?.();
    } else {
      for (const { url } of stashedFiles) URL.revokeObjectURL(url);
    }
    this.#dependencies.roomUnreadStore.setRoomUnread(post.roomId, false);
  }

  #handleEditSuccess(): void {
    this.#resetEditor();
    this.editState.cancelEdit();
  }

  async #createMessage(): Promise<void> {
    const bodyToSend = bodyForSend(this.message);
    const filesToSend = this.hasSendableAttachments ? [...this.attachments.selectedFiles] : null;
    if (!hasVisibleContent(bodyToSend) && !filesToSend) return;

    const post: PreparedPost = {
      draftKey: this.draftKey,
      roomId: this.#dependencies.getRoomId(),
      bodyToSend,
      filesToSend,
      threadRootEventId: this.#dependencies.getThreadRootEventId() ?? null,
      inReplyTo: this.#dependencies.getReplyEventId() ?? null,
      linkPreviewToken: this.linkPreviews.buildToken(),
      alsoSendToChannel: this.alsoSendToChannel,
      createThread:
        this.#dependencies.getCreateThreadRequired() ||
        (this.#dependencies.getCanCreateThread() &&
          !this.#dependencies.getThreadRootEventId() &&
          this.createThread)
    };
    const candidate =
      post.threadRootEventId === null && post.inReplyTo === null
        ? this.#dependencies.getRecentThreadRootCandidate()
        : null;
    if (candidate) {
      this.pendingThreadDestinationConfirmation = { post, candidate };
      return;
    }
    await this.submission.requestPost(post);
  }

  async #editMessage(): Promise<void> {
    const body = bodyForSend(this.message);
    if (!body) {
      toast.error('Message cannot be empty');
      return;
    }
    const eventId = this.editState.eventId;
    if (!eventId) return;
    const input: UpdateMessageInput = {
      roomId: this.#dependencies.getRoomId(),
      eventId,
      body
    };
    if (this.showEditEchoToggle) input.alsoSendToChannel = this.alsoSendToChannel;
    await this.submission.editMessage(input);
  }

  #handleEnter(event: KeyboardEvent): boolean {
    if (event.isComposing) return false;
    if (event.shiftKey || event.altKey) return false;

    const usesModifier = event.metaKey || event.ctrlKey;
    if (this.#dependencies.getComposerSendMode() === 'modifier-enter') {
      if (!usesModifier) return false;
      if (this.canSubmit) void this.submit();
      return true;
    }

    if (usesModifier) {
      this.editorApi?.performEnter();
      return true;
    }
    if (prefersTouchActions()) return false;
    if (this.canSubmit) void this.submit();
    return true;
  }

  #handleEscape(): boolean {
    if (this.isEditing) {
      this.cancelEdit();
      return true;
    }
    const callbacks = this.#dependencies.getCallbacks();
    if (this.#dependencies.getReplyEventId() && callbacks.onCancelReply) {
      callbacks.onCancelReply();
      return true;
    }
    if (callbacks.onEscape) {
      callbacks.onEscape();
      return true;
    }
    return false;
  }

  #editLastMessage(): boolean {
    if (
      this.isEditing ||
      (this.editorApi?.getText() ?? '').trim() !== '' ||
      !this.#dependencies.context.lastEditableMessage
    ) {
      return false;
    }
    const message = this.#dependencies.context.lastEditableMessage.getLastEditableMessage();
    if (!message) return false;
    this.editState.startEdit(message.eventId, message.body, {
      threadRootEventId: message.threadRootEventId,
      channelEchoEventId: message.channelEchoEventId,
      canAddChannelEcho: message.canAddChannelEcho
    });
    return true;
  }
}
