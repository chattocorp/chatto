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
import { prefersTouchActions } from '$lib/utils/inputCapabilities';
import { shouldAutoFocus } from '$lib/utils/shouldAutoFocus';
import { useDebounce } from '$lib/hooks/useDebounce.svelte';
import { toast } from '$lib/ui/toast';
import { m } from '$lib/i18n/messages';
import { AttachmentsState } from './attachments.svelte';
import { AutocompleteState, type MentionRole } from './autocomplete.svelte';
import { DraftState, draftKey } from './draft.svelte';
import type { ComposerFormattingState, TipTapEditorApi } from './editorTypes';
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

export type MessageComposerProps = {
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
  onMessageSent?: (event: TimelineEventView | null) => void;
  onCancelReply?: () => void;
  onEscape?: () => void;
  showAlsoSendToChannel?: boolean;
  showCreateThread?: boolean;
};

type MessageComposerDependencies = {
  getRoomId: () => string;
  getThreadRootEventId: () => string | undefined;
  getReplyEventId: () => string | undefined;
  getCanPost: () => boolean;
  getCanAttach: () => boolean;
  getCanCreateThread: () => boolean;
  getAutoFocus: () => boolean;
  getPlaceholder: () => string | undefined;
  getOnReady: () => MessageComposerProps['onReady'];
  getCallbacks: () => Pick<
    MessageComposerProps,
    'onTyping' | 'onMessageSent' | 'onCancelReply' | 'onEscape'
  >;
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
  editorApi = $state<TipTapEditorApi | null>(null);
  fileInputElement = $state<HTMLInputElement>();
  formattingState = $state<ComposerFormattingState>({ ...emptyFormattingState });
  alsoSendToChannel = $state(false);
  createThread = $state(false);
  editorNextEnterWillSend = $state(false);
  manualRichMode = $state(false);
  editorHasRichStructure = $state(false);
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
      onEditSuccess: () => this.#handleEditSuccess()
    });

    void dependencies.mentionRolesStore.load();
    this.#synchronizeMentionSearch();
    this.#synchronizeEditState();
    this.#synchronizeDraft();
    this.#synchronizeDraftText();
    this.#synchronizeLinkPreviews();
    this.#synchronizeAttachmentPermission();
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
      !this.#dependencies.getCanPost() ||
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
      this.attachments.pendingCount === 0 &&
      (hasVisibleContent(this.message) || this.hasSendableAttachments || this.isEditing)
    );
  }

  get isRichComposer(): boolean {
    return this.manualRichMode || this.editorHasRichStructure;
  }

  get nextEnterWillSend(): boolean {
    return this.canSubmit && this.isRichComposer && this.editorNextEnterWillSend;
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
      this.inputDisabled ||
      this.attachments.pendingCount > 0
    ) {
      return;
    }
    await (this.isEditing ? this.#editMessage() : this.#createMessage());
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
    if (event.key === 'Enter' && !event.ctrlKey && !event.metaKey && prefersTouchActions()) {
      return false;
    }
    if (event.key === 'Enter' && !event.shiftKey && this.#handleEnter(event)) return true;
    if (event.key === 'Tab' && this.autocomplete.handleTabCompletion(event)) return true;
    if (event.key !== 'Tab') this.autocomplete.resetTabCompletion();
    if (event.key === 'Escape' && this.#handleEscape()) return true;
    if (event.key === 'ArrowUp' && this.#editLastMessage()) return true;
    return false;
  }

  handleEditorUpdate(text: string): void {
    const changed = text !== this.message;
    this.message = text;
    if (!text) this.manualRichMode = false;
    if (changed) this.#dependencies.getCallbacks().onTyping?.();
    this.autocomplete.update();
  }

  handleEditorReady(api: TipTapEditorApi): void {
    this.editorApi = api;
    if (this.message) api.setContent(this.message);
    if (this.#focusRequested) this.focus();
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
        this.manualRichMode = false;
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
        this.createThread = false;
      }
      if (this.isEditing) {
        this.draft.switchKey(this.draftKey);
        this.attachments.restore([]);
        return;
      }
      const draftMessage = this.draft.switchKey(this.draftKey);
      this.message = draftMessage;
      this.manualRichMode = false;
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
    this.manualRichMode = false;
    this.alsoSendToChannel = false;
    this.createThread = false;
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
      this.#dependencies.getCallbacks().onMessageSent?.(event);
      this.#dependencies.context.scrollState?.requestScrollToBottom();
      this.#dependencies.getCallbacks().onCancelReply?.();
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

    await this.submission.requestPost({
      draftKey: this.draftKey,
      roomId: this.#dependencies.getRoomId(),
      bodyToSend,
      filesToSend,
      threadRootEventId: this.#dependencies.getThreadRootEventId() ?? null,
      inReplyTo: this.#dependencies.getReplyEventId() ?? null,
      linkPreviewToken: this.linkPreviews.buildToken(),
      alsoSendToChannel: this.alsoSendToChannel,
      createThread:
        this.#dependencies.getCanCreateThread() &&
        !this.#dependencies.getThreadRootEventId() &&
        this.createThread
    });
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
    if (event.metaKey || event.ctrlKey) {
      if (this.isRichComposer) {
        void this.submit();
      } else {
        if (hasVisibleContent(this.message)) this.editorApi?.insertBlockBreak();
        this.manualRichMode = true;
      }
      return true;
    }
    if (!this.isRichComposer && this.canSubmit) {
      void this.submit();
      return true;
    }
    if (this.isRichComposer && this.nextEnterWillSend) {
      void this.submit();
      return true;
    }
    return false;
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
