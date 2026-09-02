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
  RealtimeInitialState,
  RealtimeRecoveryMode,
  RealtimeServerFrame,
  RealtimeSubscribeEvents
} from '@chatto/api-types/realtime/v1/realtime_pb';
import { ListNotificationOccurrencesResponse } from '@chatto/api-types/api/v1/notifications_pb';

interface RealtimeConnectOptions {
  initialState?: RealtimeInitialState;
  resumeCursor?: string;
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
          value: new RealtimeClientHello({ protocolVersion: 4, bearerToken })
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
            resumeCursor: options.resumeCursor
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
              return `event:${frame.frame.value.event?.event.case ?? 'unknown'}`;
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
  test('hands an exact bounded snapshot to subsequent realtime events', async ({
    page,
    browser,
    serverURL
  }) => {
    const viewer = await createAndLoginTestUser(page);
    const roomId = await getRoomIdByNameViaConnect(page, 'general');
    const token = await loginForBearerToken(page, viewer);
    let unrelatedUserId: string | undefined;
    await withServerUser(browser!, serverURL, async ({ user }) => {
      unrelatedUserId = user.id;
    });

    const realtime = await RealtimeProtobufClient.connect(serverURL, token);
    try {
      expect(realtime.recoveryMode).toBe(RealtimeRecoveryMode.SNAPSHOT);

      // The snapshot boundary is already captured when subscribed arrives.
      // This message must therefore arrive after caught_up, even if the
      // snapshot frames are still in flight.
      const messageId = await postMessageViaConnect(
        page,
        roomId,
        `snapshot boundary ${Date.now()}`
      );

      const server = await realtime.waitForFrame(
        (frame) => frame.frame.case === 'snapshot' && frame.frame.value.resource.case === 'server'
      );
      expect(server.frame.case).toBe('snapshot');

      const rooms = await realtime.waitForFrame(
        (frame) => frame.frame.case === 'snapshot' && frame.frame.value.resource.case === 'rooms'
      );
      if (rooms.frame.case !== 'snapshot' || rooms.frame.value.resource.case !== 'rooms') {
        throw new Error('expected rooms snapshot');
      }
      expect(
        rooms.frame.value.resource.value.rooms.some((room) => room.room?.id === roomId)
      ).toBe(true);

      await realtime.waitForFrame(
        (frame) =>
          frame.frame.case === 'snapshot' && frame.frame.value.resource.case === 'roomGroups'
      );
      const users = await realtime.waitForFrame(
        (frame) => frame.frame.case === 'snapshot' && frame.frame.value.resource.case === 'users'
      );
      if (users.frame.case !== 'snapshot' || users.frame.value.resource.case !== 'users') {
        throw new Error('expected users snapshot');
      }
      expect(users.frame.value.resource.value.users.some((user) => user.user?.id === viewer.id)).toBe(
        true
      );
      expect(
        users.frame.value.resource.value.users.some((user) => user.user?.id === unrelatedUserId)
      ).toBe(false);

      await realtime.waitForFrame(
        (frame) =>
          frame.frame.case === 'snapshot' && frame.frame.value.resource.case === 'activeCalls'
      );
      const caughtUp = await realtime.waitForFrame((frame) => frame.frame.case === 'caughtUp');
      if (caughtUp.frame.case !== 'caughtUp') throw new Error('expected caught_up frame');
      expect(caughtUp.frame.value.cursor).toBeTruthy();

      const event = await realtime.waitForEvent((candidate) => candidate.event?.id === messageId);
      expect(event.resumeCursor).toBeTruthy();
      expect(event.resumeCursor).not.toBe(caughtUp.frame.value.cursor);
    } finally {
      realtime.close();
    }
  });

  test('bundled resource bootstrap does not list the complete user directory', async ({
    page,
    chatPage
  }) => {
    let fullUserDirectoryReads = 0;
    page.on('request', (request) => {
      if (request.url().includes('chatto.api.v1.UserService/ListUsers')) {
        fullUserDirectoryReads++;
      }
    });
    await createAndLoginTestUser(page);

    await chatPage.goto();
    await expect(chatPage.getRoomLink('general')).toBeVisible();

    expect(fullUserDirectoryReads).toBe(0);
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
        (event) => event.event?.event.case === 'messagePosted' && event.event.id === messageId
      );
      expect(posted.resumeCursor).toBeTruthy();
      if (posted.event?.event.case !== 'messagePosted') {
        throw new Error('expected canonical message-post event');
      }
      expect(posted.event.event.value.bodyPlaintext).toContain('semantic realtime');
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
            event.event?.event.case === 'reactionAdded' &&
            event.event.event.value.messageEventId === messageId
        );
        expect(reaction.event?.event.case).toBe('reactionAdded');
        expect(reaction.resumeCursor).toBeTruthy();
        await resumed.waitForFrame((frame) => frame.frame.case === 'caughtUp');

        await connectPost(page, 'chatto.api.v1.MessageService/UpdateMessage', {
          roomId,
          eventId: messageId,
          body: `edited semantic realtime ${Date.now()}`
        });
        const edited = await resumed.waitForEvent(
          (event) =>
            event.event?.event.case === 'messageEdited' &&
            event.event.event.value.eventId === messageId
        );
        expect(edited.resumeCursor).toBeTruthy();
      } finally {
        resumed.close();
      }
    } finally {
      realtime.close();
    }
  });

  test('uses canonical events as notification resource refresh hints', async ({
    page,
    browser,
    serverURL
  }) => {
    const viewer = await createAndLoginTestUser(page);
    const token = await loginForBearerToken(page, viewer);
    const realtime = await RealtimeProtobufClient.connect(serverURL, token, {
      initialState: RealtimeInitialState.LIVE_ONLY
    });

    try {
      let mentionActorDisplayName = '';
      await withServerUser(browser!, serverURL, async ({ user, chatPage, roomPage }) => {
        mentionActorDisplayName = user.displayName;
        await chatPage.enterRoom('general');
        await roomPage.sendMessage(`@${viewer.login} protobuf mention ${Date.now()}`);
      });

      await realtime.waitForEvent((event) => event.event?.event.case === 'messagePosted');
      await expect
        .poll(async () => {
          const json = await connectPost<Record<string, unknown>>(
            page,
            'chatto.api.v1.NotificationService/ListNotificationOccurrences'
          );
          const response = ListNotificationOccurrencesResponse.fromJson(json);
          const occurrence = response.occurrences.find(
            (item) => item.signal?.kind.case === 'directMentionReceived'
          );
          const message =
            occurrence?.signal?.kind.case === 'directMentionReceived'
              ? occurrence.signal.kind.value.message
              : undefined;
          return {
            actorId: occurrence?.actor?.id,
            actorDisplayName: occurrence?.actor?.displayName,
            roomId: message?.room?.id,
            roomName: message?.room?.name
          };
        })
        .toMatchObject({
          actorId: expect.any(String),
          actorDisplayName: mentionActorDisplayName,
          roomId: expect.any(String),
          roomName: 'general'
        });

      let dmSenderDisplayName = '';
      await withServerUser(browser!, serverURL, async ({ user, page: senderPage }) => {
        dmSenderDisplayName = user.displayName;
        const dmPage = new DMPage(senderPage);
        const roomPage = await dmPage.startConversation(viewer.login);
        await roomPage.sendMessage(`protobuf dm ${Date.now()}`);
      });

      await realtime.waitForEvent((event) => event.event?.event.case === 'messagePosted');
      await expect
        .poll(async () => {
          const json = await connectPost<Record<string, unknown>>(
            page,
            'chatto.api.v1.NotificationService/ListNotificationOccurrences'
          );
          const response = ListNotificationOccurrencesResponse.fromJson(json);
          const occurrence = response.occurrences.find(
            (item) => item.signal?.kind.case === 'directMessageReceived'
          );
          const message =
            occurrence?.signal?.kind.case === 'directMessageReceived'
              ? occurrence.signal.kind.value.message
              : undefined;
          return {
            actorId: occurrence?.actor?.id,
            actorDisplayName: occurrence?.actor?.displayName,
            roomId: message?.room?.id
          };
        })
        .toMatchObject({
          actorId: expect.any(String),
          actorDisplayName: dmSenderDisplayName,
          roomId: expect.any(String)
        });
    } finally {
      realtime.close();
    }
  });
});
