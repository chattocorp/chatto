import type { Page } from '@playwright/test';
import { test, expect } from './setup';
import { DMPage } from './pages';
import { createAndLoginTestUser, type TestUser } from './fixtures/testUser';
import { withServerUser } from './fixtures/serverUser';
import {
  connectPost,
  getRoomIdByNameViaConnect,
  postMessageViaConnect
} from './fixtures/connectHelpers';
import { TIMEOUTS } from './constants';
import {
  RealtimeClientFrame,
  RealtimeClientHello,
  RealtimeEvent,
  RealtimeHydrateRoom,
  RealtimeInitialState,
  RealtimeMessageAction,
  RealtimeReactionAction,
  RealtimeRecoveryMode,
  RealtimeServerFrame,
  RealtimeStateItem,
  RealtimeSubscribeEvents
} from '@chatto/api-types/realtime/v1/realtime_pb';

interface RealtimeConnectOptions {
  initialState?: RealtimeInitialState;
  resumeCursor?: string;
  retainedRoomIds?: string[];
}

class RealtimeProtobufClient {
  readonly #socket: WebSocket;
  readonly #frames: RealtimeServerFrame[] = [];
  readonly #waiters: Array<{
    predicate: (frame: RealtimeServerFrame) => boolean;
    resolve: (frame: RealtimeServerFrame) => void;
    reject: (err: Error) => void;
    timer: ReturnType<typeof setTimeout>;
  }> = [];
  recoveryMode: RealtimeRecoveryMode;

  private constructor(socket: WebSocket, recoveryMode: RealtimeRecoveryMode) {
    this.#socket = socket;
    this.recoveryMode = recoveryMode;
    socket.addEventListener('message', (message) => {
      void this.#handleMessage(message.data);
    });
    socket.addEventListener('close', () => {
      this.#rejectAll(new Error('realtime socket closed'));
    });
    socket.addEventListener('error', () => {
      this.#rejectAll(new Error('realtime socket error'));
    });
  }

  static async connect(
    serverURL: string,
    bearerToken: string,
    options: RealtimeConnectOptions = {}
  ): Promise<RealtimeProtobufClient> {
    const url = new URL('/api/realtime', serverURL);
    url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';

    const socket = new WebSocket(url);
    socket.binaryType = 'arraybuffer';
    await new Promise<void>((resolve, reject) => {
      const timer = setTimeout(() => reject(new Error('timed out opening realtime socket')), 5000);
      socket.addEventListener(
        'open',
        () => {
          clearTimeout(timer);
          resolve();
        },
        { once: true }
      );
      socket.addEventListener(
        'error',
        () => {
          clearTimeout(timer);
          reject(new Error('failed to open realtime socket'));
        },
        { once: true }
      );
    });

    const pendingClient = new RealtimeProtobufClient(socket, RealtimeRecoveryMode.UNSPECIFIED);
    pendingClient.send(
      new RealtimeClientFrame({
        frame: {
          case: 'hello',
          value: new RealtimeClientHello({ protocolVersion: 3, bearerToken })
        }
      })
    );
    await pendingClient.waitForFrame((frame) => frame.frame.case === 'hello');
    pendingClient.send(
      new RealtimeClientFrame({
        frame: {
          case: 'subscribeEvents',
          value: new RealtimeSubscribeEvents({
            initialState: options.initialState ?? RealtimeInitialState.SNAPSHOT,
            resumeCursor: options.resumeCursor,
            retainedRoomIds: options.retainedRoomIds ?? []
          })
        }
      })
    );
    const subscribed = await pendingClient.waitForFrame(
      (frame) => frame.frame.case === 'subscribed'
    );
    if (subscribed.frame.case !== 'subscribed') throw new Error('expected subscribed frame');
    pendingClient.recoveryMode = subscribed.frame.value.recoveryMode;
    return pendingClient;
  }

  close(): void {
    this.#socket.close();
    this.#rejectAll(new Error('realtime socket closed'));
  }

  send(frame: RealtimeClientFrame): void {
    this.#socket.send(frame.toBinary());
  }

  waitForEvent(predicate: (event: RealtimeEvent) => boolean): Promise<RealtimeEvent> {
    return this.waitForFrame((frame) => {
      const event = frame.frame.case === 'event' ? frame.frame.value : null;
      return event ? predicate(event) : false;
    }).then((frame) => {
      if (frame.frame.case !== 'event') throw new Error('matched frame was not an event');
      return frame.frame.value;
    });
  }

  waitForState(predicate: (state: RealtimeStateItem) => boolean): Promise<RealtimeStateItem> {
    return this.waitForFrame((frame) => {
      const state = frame.frame.case === 'state' ? frame.frame.value : null;
      return state ? predicate(state) : false;
    }).then((frame) => {
      if (frame.frame.case !== 'state') throw new Error('matched frame was not state');
      return frame.frame.value;
    });
  }

  waitForFrame(predicate: (frame: RealtimeServerFrame) => boolean): Promise<RealtimeServerFrame> {
    const queuedIndex = this.#frames.findIndex(predicate);
    if (queuedIndex >= 0) {
      const [frame] = this.#frames.splice(queuedIndex, 1);
      return Promise.resolve(frame);
    }

    return new Promise((resolve, reject) => {
      const waiter = {
        predicate,
        resolve,
        reject,
        timer: setTimeout(() => {
          const index = this.#waiters.indexOf(waiter);
          if (index >= 0) this.#waiters.splice(index, 1);
          const queued = this.#frames.map((frame) => {
            if (frame.frame.case === 'event') {
              return `event:${frame.frame.value.event.case ?? 'unknown'}`;
            }
            if (frame.frame.case === 'state') {
              return `state:${frame.frame.value.state.case ?? 'unknown'}`;
            }
            return frame.frame.case ?? 'unknown';
          });
          reject(new Error(`timed out waiting for realtime frame; queued: ${queued.join(', ')}`));
        }, TIMEOUTS.REALTIME_EVENT)
      };
      this.#waiters.push(waiter);
    });
  }

  async #handleMessage(data: unknown): Promise<void> {
    const frame = RealtimeServerFrame.fromBinary(await websocketDataToBytes(data));
    if (frame.frame.case === 'close') {
      this.#rejectAll(
        new Error(`realtime server closed: ${frame.frame.value.code}: ${frame.frame.value.message}`)
      );
    } else if (frame.frame.case === 'error' && frame.frame.value.fatal) {
      this.#rejectAll(
        new Error(`realtime server error: ${frame.frame.value.code}: ${frame.frame.value.message}`)
      );
    }
    const waiterIndex = this.#waiters.findIndex((waiter) => waiter.predicate(frame));
    if (waiterIndex >= 0) {
      const [waiter] = this.#waiters.splice(waiterIndex, 1);
      clearTimeout(waiter.timer);
      waiter.resolve(frame);
      return;
    }
    this.#frames.push(frame);
  }

  #rejectAll(err: Error): void {
    for (const waiter of this.#waiters.splice(0)) {
      clearTimeout(waiter.timer);
      waiter.reject(err);
    }
  }
}

async function websocketDataToBytes(data: unknown): Promise<Uint8Array> {
  if (data instanceof Uint8Array) return data;
  if (data instanceof ArrayBuffer) return new Uint8Array(data);
  if (data instanceof Blob) return new Uint8Array(await data.arrayBuffer());
  throw new Error(`unsupported realtime message data: ${typeof data}`);
}

async function loginForBearerToken(page: Page, user: TestUser): Promise<string> {
  const loginResponse = await page.request.post('/auth/login', {
    data: { login: user.login, password: user.password }
  });
  expect(loginResponse.ok()).toBeTruthy();
  const loginData = await loginResponse.json();
  expect(loginData.token).toBeTruthy();
  return loginData.token as string;
}

test.describe('protobuf realtime stream', () => {
  test('materialises a cold room timeline through explicit snapshot state', async ({
    page,
    chatPage,
    serverURL
  }) => {
    const viewer = await createAndLoginTestUser(page);
    await chatPage.goto();
    const roomId = await getRoomIdByNameViaConnect(page, 'general');
    const messageId = await postMessageViaConnect(page, roomId, `lazy realtime ${Date.now()}`);
    const token = await loginForBearerToken(page, viewer);
    const realtime = await RealtimeProtobufClient.connect(serverURL, token);

    try {
      expect(realtime.recoveryMode).toBe(RealtimeRecoveryMode.SNAPSHOT);
      await realtime.waitForFrame((frame) => frame.frame.case === 'caughtUp');
      realtime.send(
        new RealtimeClientFrame({
          frame: {
            case: 'hydrateRoom',
            value: new RealtimeHydrateRoom({ roomId })
          }
        })
      );

      const hydrated = await realtime.waitForState(
        (state) =>
          state.state.case === 'roomTimeline' &&
          state.state.value.roomId === roomId &&
          Boolean(state.state.value.page?.events.some((event) => event.id === messageId))
      );
      expect(hydrated.state.case).toBe('roomTimeline');
    } finally {
      realtime.close();
    }
  });

  test('delivers unretained semantic events and resumes them from a cursor', async ({
    page,
    serverURL
  }) => {
    const viewer = await createAndLoginTestUser(page);
    const roomId = await getRoomIdByNameViaConnect(page, 'general');
    const token = await loginForBearerToken(page, viewer);
    const realtime = await RealtimeProtobufClient.connect(serverURL, token, {
      initialState: RealtimeInitialState.LIVE_ONLY
    });

    try {
      expect(realtime.recoveryMode).toBe(RealtimeRecoveryMode.LIVE_ONLY);
      await realtime.waitForFrame((frame) => frame.frame.case === 'caughtUp');
      const messageId = await postMessageViaConnect(
        page,
        roomId,
        `semantic realtime ${Date.now()}`
      );
      const posted = await realtime.waitForEvent(
        (event) =>
          event.event.case === 'message' &&
          event.event.value.action === RealtimeMessageAction.POSTED &&
          event.event.value.messageEventId === messageId
      );
      expect(posted.resumeCursor).toBeTruthy();
      expect(posted.state.some((state) => state.state.case === 'roomTimelineEvent')).toBe(false);
      realtime.close();

      await connectPost(page, 'chatto.api.v1.MessageService/AddReaction', {
        roomId,
        messageEventId: messageId,
        emoji: 'thumbsup'
      });

      const resumed = await RealtimeProtobufClient.connect(serverURL, token, {
        initialState: RealtimeInitialState.LIVE_ONLY,
        resumeCursor: posted.resumeCursor
      });
      try {
        expect(resumed.recoveryMode).toBe(RealtimeRecoveryMode.RESUME);
        const reaction = await resumed.waitForEvent(
          (event) =>
            event.event.case === 'reaction' &&
            event.event.value.action === RealtimeReactionAction.ADDED &&
            event.event.value.messageEventId === messageId
        );
        expect(reaction.event.case).toBe('reaction');
        expect(reaction.resumeCursor).toBeTruthy();
        await resumed.waitForFrame((frame) => frame.frame.case === 'caughtUp');

        await connectPost(page, 'chatto.api.v1.MessageService/UpdateMessage', {
          roomId,
          eventId: messageId,
          body: `edited semantic realtime ${Date.now()}`
        });
        const edited = await resumed.waitForEvent(
          (event) =>
            event.event.case === 'message' &&
            event.event.value.action === RealtimeMessageAction.EDITED &&
            event.event.value.messageEventId === messageId
        );
        expect(edited.resumeCursor).toBeTruthy();
      } finally {
        resumed.close();
      }
    } finally {
      realtime.close();
    }
  });

  test('delivers mention and DM occurrence display payloads as event state', async ({
    page,
    browser,
    serverURL
  }) => {
    const viewer = await createAndLoginTestUser(page);
    const token = await loginForBearerToken(page, viewer);
    const realtime = await RealtimeProtobufClient.connect(serverURL, token);

    try {
      let mentionActorDisplayName = '';
      await withServerUser(browser!, serverURL, async ({ user, chatPage, roomPage }) => {
        mentionActorDisplayName = user.displayName;
        await chatPage.enterRoom('general');
        await roomPage.sendMessage(`@${viewer.login} protobuf mention ${Date.now()}`);
      });

      const mentionEvent = await realtime.waitForEvent((event) =>
        event.state.some(
          (state) =>
            state.state.case === 'notifications' &&
            state.state.value.occurrences?.occurrences.some(
              (occurrence) => occurrence.signal?.kind.case === 'directMentionReceived'
            )
        )
      );
      const mentionReplacement = mentionEvent.state
        .map((state) => (state.state.case === 'notifications' ? state.state.value : null))
        .find((replacement) => replacement?.occurrences?.occurrences.length);
      const mention = mentionReplacement?.occurrences?.occurrences.find(
        (occurrence) => occurrence.signal?.kind.case === 'directMentionReceived'
      );
      expect(mention?.actor?.displayName).toBe(mentionActorDisplayName);
      expect(mention?.actor?.id).toBeTruthy();
      const mentionMessage =
        mention?.signal?.kind.case === 'directMentionReceived'
          ? mention.signal.kind.value.message
          : null;
      expect(mentionMessage?.room?.name).toBe('general');
      expect(mentionMessage?.room?.id).toBeTruthy();

      let dmSenderDisplayName = '';
      await withServerUser(browser!, serverURL, async ({ user, page: senderPage }) => {
        dmSenderDisplayName = user.displayName;
        const dmPage = new DMPage(senderPage);
        const roomPage = await dmPage.startConversation(viewer.login);
        await roomPage.sendMessage(`protobuf dm ${Date.now()}`);
      });

      const dmEvent = await realtime.waitForEvent((event) =>
        event.state.some(
          (state) =>
            state.state.case === 'notifications' &&
            state.state.value.occurrences?.occurrences.some(
              (occurrence) => occurrence.signal?.kind.case === 'directMessageReceived'
            )
        )
      );
      const dmReplacement = dmEvent.state
        .map((state) => (state.state.case === 'notifications' ? state.state.value : null))
        .find((replacement) => replacement?.occurrences?.occurrences.length);
      const dm = dmReplacement?.occurrences?.occurrences.find(
        (occurrence) => occurrence.signal?.kind.case === 'directMessageReceived'
      );
      expect(dm?.actor?.displayName).toBe(dmSenderDisplayName);
      expect(dm?.actor?.id).toBeTruthy();
      const dmMessage =
        dm?.signal?.kind.case === 'directMessageReceived' ? dm.signal.kind.value.message : null;
      expect(dmMessage?.room?.id).toBeTruthy();
    } finally {
      realtime.close();
    }
  });
});
