import type { TimelineEventView } from '@chatto/client/timeline/timelineEvents';
import { isMessagePostedEvent } from '@chatto/client/timeline/timelineEvents';
import { RoomThreadingMode } from '@chatto/client/util/roomThreading';
import { useMessageActions } from '$lib/hooks';
import { m } from '$lib/i18n/messages';
import {
  getComposerContext,
  useRoomPermissions,
  type MessagesStore,
  type QuoteInsertionContent
} from '$lib/state/room';
import { useServerScope } from '$lib/state/server/scope.svelte';
import { toast } from '$lib/ui/toast';
import { buildMessageActionModel } from './messageActionModel';
import {
  canEditMessage,
  resolveMessageAuthor,
  resolveMessageEventReferences
} from './messageEventModel';
import { roomReplyTargetEventId } from './messageReplyTarget';
import type { OpenThreadHandler } from './threadOpenOptions';

/** The inputs of one message action target. Read again on every reactive update. */
export type MessageActionTargetInput = {
  event: TimelineEventView;
  roomId: string;
  /** Thread root of the pane that shows the message, or null in the room timeline. */
  permalinkThreadRootEventId: string | null;
  messageStore: MessagesStore | null;
  onOpenThread?: OpenThreadHandler;
  threadingMode: RoomThreadingMode;
  /** Returns the selected text to quote in the next reply, and clears it. */
  takeReplyQuote: () => QuoteInsertionContent | null;
};

/**
 * Binds one timeline message to its actions: permissions, reply and thread routing, pins,
 * and the `MessageActionModel` that every action surface uses.
 *
 * The message row creates one for its toolbar and footer. The message action overlay host
 * creates one for the message that owns the open overlay, so the overlay keeps working
 * after the virtualizer unmounts the row.
 *
 * Construct it during component initialization: it captures the server scope, the room
 * permissions, and the composer context.
 */
export class MessageActionTarget {
  readonly #input: () => MessageActionTargetInput;
  readonly #scope = useServerScope();
  readonly #stores = this.#scope.store;
  readonly #permissions = useRoomPermissions();
  readonly #composer = getComposerContext();
  readonly #actions = useMessageActions();

  constructor(input: () => MessageActionTargetInput) {
    this.#input = input;
  }

  readonly #in = $derived.by(() => this.#input());
  readonly event = $derived(this.#in.event);
  /** The posted-message payload, or null during virtualizer data transitions. */
  readonly messageEvent = $derived(
    isMessagePostedEvent(this.event?.event) ? this.event.event : null
  );
  readonly permissions = $derived.by(() => this.#permissions());

  // Resolve against the live profile owner. Timeline includes can arrive before
  // profiles catch up after a reconnect.
  readonly #author = $derived(resolveMessageAuthor(this.event, this.#stores.projection.users));
  readonly actor = $derived(this.#author.user);
  readonly deletedActor = $derived(this.#author.deleted);
  /** The actor already uses the live profile when one is available. */
  readonly displayName = $derived(
    this.actor
      ? this.actor.displayName || this.actor.login
      : this.deletedActor
        ? m('common.deleted_user')
        : m('common.unknown_user')
  );

  // Authors can always edit (within the edit window) and delete their own messages;
  // managing other users' messages requires message.manage.
  readonly isAuthor = $derived(this.#stores.viewerId === this.event?.actorId);
  readonly canEdit = $derived(
    canEditMessage({
      isAuthor: this.isAuthor,
      createdAt: this.event.createdAt,
      now: Date.now(),
      editWindowSeconds: this.#stores.serverInfo.messageEditWindowSeconds,
      canManageOthersMessage: this.permissions.canManageOthersMessage
    })
  );
  readonly canDelete = $derived(this.isAuthor || this.permissions.canManageOthersMessage);

  readonly #references = $derived(
    this.messageEvent ? resolveMessageEventReferences(this.event.id, this.messageEvent) : null
  );
  readonly isEcho = $derived(this.#references?.isEcho ?? false);
  readonly #editEventId = $derived(this.#references?.editEventId ?? this.event.id);
  readonly #editThreadRootEventId = $derived(this.#references?.editThreadRootEventId ?? null);
  readonly #editChannelEchoEventId = $derived(this.#references?.editChannelEchoEventId ?? null);
  readonly threadRootEventId = $derived(this.#references?.threadRootEventId ?? null);

  readonly #pinsStore = $derived(
    this.permissions.canViewPinnedMessages ? this.#stores.rooms.pins(this.#in.roomId) : null
  );
  readonly isPinned = $derived(
    this.#pinsStore?.isPinned(this.#editEventId, this.messageEvent?.pinned ?? false) ??
      this.messageEvent?.pinned ??
      false
  );

  // Threading uses threadRootEventId (thread membership), not inReplyTo (attribution).
  // Echoes never have replies.
  readonly isRootMessage = $derived(!this.isEcho && this.messageEvent?.threadRootEventId == null);
  readonly hasReplies = $derived(this.isRootMessage && (this.messageEvent?.replyCount ?? 0) > 0);
  readonly hasThread = $derived(
    this.isRootMessage &&
      ((this.messageEvent?.threadExists ?? false) || (this.messageEvent?.replyCount ?? 0) > 0)
  );
  readonly isInThreadPane = $derived(!!this.#in.permalinkThreadRootEventId);
  readonly isEchoedToChannel = $derived(
    this.isInThreadPane && !this.isEcho && !!this.messageEvent?.channelEchoEventId
  );
  readonly #canReplyInThread = $derived(
    this.messageEvent?.canReplyInThread ?? this.permissions.canPostInThread
  );

  readonly #canUseReplyAction = $derived.by(() => {
    const { threadingMode, onOpenThread } = this.#in;
    if (threadingMode === RoomThreadingMode.DISABLED && this.isInThreadPane) return false;
    if (this.isEcho) {
      return (
        threadingMode !== RoomThreadingMode.DISABLED &&
        this.#canReplyInThread &&
        !!onOpenThread &&
        !!this.messageEvent?.echoFromThreadRootEventId
      );
    }
    if (this.isInThreadPane) return this.#canReplyInThread;
    if (this.isRootMessage && threadingMode === RoomThreadingMode.REQUIRED) {
      return this.#canReplyInThread && !!onOpenThread;
    }
    if (this.isRootMessage && threadingMode === RoomThreadingMode.ENCOURAGED) {
      return (this.#canReplyInThread && !!onOpenThread) || this.permissions.canPostMessage;
    }
    return this.permissions.canPostMessage;
  });
  readonly #canUseSecondaryRoomReply = $derived(
    this.isRootMessage &&
      !this.isInThreadPane &&
      this.#in.threadingMode === RoomThreadingMode.ENCOURAGED &&
      this.#canReplyInThread &&
      !!this.#in.onOpenThread &&
      this.permissions.canPostMessage
  );
  readonly #canUseThreadAction = $derived.by(() => {
    const { threadingMode, onOpenThread, permalinkThreadRootEventId } = this.#in;
    if (this.isEcho) return !!onOpenThread && !!this.messageEvent?.echoFromThreadRootEventId;
    if (threadingMode === RoomThreadingMode.DISABLED) {
      return !permalinkThreadRootEventId && this.isRootMessage && this.hasThread && !!onOpenThread;
    }
    return this.#canReplyInThread && !!onOpenThread;
  });

  /** The behavior that every message action surface shares for this message. */
  readonly action = $derived.by(() => {
    const { roomId, permalinkThreadRootEventId, messageStore, threadingMode } = this.#in;
    const canAddChannelEcho =
      this.isAuthor &&
      !!this.#editThreadRootEventId &&
      threadingMode !== RoomThreadingMode.DISABLED &&
      this.permissions.canEchoMessage &&
      this.permissions.canPostMessage;
    const canRemoveChannelEcho =
      (this.isAuthor || this.permissions.canManageOthersMessage) &&
      !!this.#editThreadRootEventId &&
      !!this.#editChannelEchoEventId;
    return buildMessageActionModel({
      actions: this.#actions,
      params: {
        serverId: this.#scope.serverId,
        roomId,
        messageEventId: this.event.id,
        eventId: this.#editEventId,
        deleteEventId: this.event.id,
        messageBody: this.messageEvent?.body ?? '',
        permalinkThreadRootEventId,
        threadRootEventId: this.#editThreadRootEventId,
        channelEchoEventId: this.#editChannelEchoEventId,
        canAddChannelEcho,
        canRemoveChannelEcho,
        messageStore
      },
      reactions: this.messageEvent?.reactions ?? [],
      canReact: this.permissions.canReact,
      canEdit: this.canEdit,
      canDelete: this.canDelete,
      canPin: this.permissions.canPinMessages && Boolean(this.#pinsStore),
      isPinned: this.isPinned,
      togglePin: () => this.#togglePin(),
      replyInRoomLabel: this.isEcho
        ? m('room.message.actions.reply_thread')
        : m('room.message.actions.reply'),
      replyThreadLabel:
        this.isEcho ||
        (this.isRootMessage && threadingMode === RoomThreadingMode.DISABLED && this.hasThread)
          ? m('room.message.actions.open_thread')
          : m('room.message.actions.reply_thread'),
      replyInRoom: this.#canUseReplyAction ? () => this.#reply() : undefined,
      replyThread: this.#canUseThreadAction
        ? this.isEcho || threadingMode === RoomThreadingMode.DISABLED
          ? () => this.openThread()
          : () => this.#startReplyInThread(this.#in.takeReplyQuote())
        : undefined,
      secondaryReplyInRoomLabel: this.#canUseSecondaryRoomReply
        ? m('room.message.actions.reply_room')
        : undefined,
      secondaryReplyInRoom: this.#canUseSecondaryRoomReply
        ? () => this.#startReplyInCurrentComposer(this.#in.takeReplyQuote())
        : undefined
    });
  });

  /** Opens the thread of this message. Echoes open the thread of their original. */
  openThread(): void {
    const { onOpenThread, permalinkThreadRootEventId, takeReplyQuote } = this.#in;
    if (!onOpenThread) return;
    const threadRoot =
      (this.isEcho ? this.messageEvent?.echoFromThreadRootEventId : null) ??
      permalinkThreadRootEventId ??
      this.event.id;
    // Opening a thread does not quote; drop the pending selection.
    takeReplyQuote();
    onOpenThread(threadRoot);
    // The thread's ConversationPane marks the thread read. That also covers
    // direct URL navigation to threads.
  }

  async #togglePin(): Promise<void> {
    const pins = this.#pinsStore;
    if (!pins) return;
    try {
      if (pins.isPinned(this.#editEventId, this.messageEvent?.pinned ?? false))
        await pins.remove(this.#editEventId);
      else await pins.create(this.#editEventId);
    } catch {
      toast.error(m('room.pins.update_failed'));
    }
  }

  #reply(): void {
    const { onOpenThread, threadingMode, takeReplyQuote } = this.#in;
    const quote = takeReplyQuote();
    const messageEvent = this.messageEvent;
    if (this.isEcho && messageEvent?.echoOfEventId && messageEvent.echoFromThreadRootEventId) {
      onOpenThread?.(messageEvent.echoFromThreadRootEventId, {
        highlightEventId: messageEvent.echoOfEventId,
        quoteText: quote ?? undefined,
        reply: {
          eventId: messageEvent.echoOfEventId,
          actorDisplayName: this.displayName,
          actorIdentity: this.actor ?? undefined,
          excerpt: messageEvent.body ?? ''
        }
      });
      return;
    }

    if (this.isInThreadPane) {
      this.#startReplyInCurrentComposer(quote);
      return;
    }

    if (
      this.isRootMessage &&
      (threadingMode === RoomThreadingMode.REQUIRED ||
        (threadingMode === RoomThreadingMode.ENCOURAGED &&
          this.#canReplyInThread &&
          !!onOpenThread))
    ) {
      this.#startReplyInThread(quote);
      return;
    }

    this.#startReplyInCurrentComposer(quote);
  }

  #startReplyInCurrentComposer(quote: QuoteInsertionContent | null): void {
    this.#composer.replyState.startReply(
      roomReplyTargetEventId(this.event),
      this.displayName,
      this.messageEvent?.body ?? '',
      this.actor ?? undefined
    );
    if (quote) this.#composer.quoteInsertionState.requestInsertQuote(quote);
  }

  #startReplyInThread(quote: QuoteInsertionContent | null): void {
    const { onOpenThread, permalinkThreadRootEventId } = this.#in;
    onOpenThread?.(permalinkThreadRootEventId ?? this.event.id, {
      quoteText: quote ?? undefined,
      reply: {
        eventId: roomReplyTargetEventId(this.event),
        actorDisplayName: this.displayName,
        actorIdentity: this.actor ?? undefined,
        excerpt: this.messageEvent?.body ?? ''
      }
    });
  }
}
