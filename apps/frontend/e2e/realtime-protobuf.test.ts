import type { Page } from '@playwright/test';
import { test, expect } from './setup';
import { DMPage } from './pages';
import { createAndLoginTestUser, type TestUser } from './fixtures/testUser';
import { withServerUser } from './fixtures/serverUser';
import { getRoomIdByNameViaConnect, postMessageViaConnect } from './fixtures/connectHelpers';
import { TIMEOUTS } from './constants';
import {
  RealtimeClientFrame,
  RealtimeClientHello,
  RealtimeEventEnvelope,
  RealtimeHydrateRoom,
  RealtimeServerFrame,
  RealtimeSubscribeEvents
} from '@chatto/api-types/realtime/v1/realtime_pb';

class RealtimeProtobufClient {
  readonly #socket: WebSocket;
  readonly #frames: RealtimeServerFrame[] = [];
  readonly #waiters: Array<{
    predicate: (frame: RealtimeServerFrame) => boolean;
    resolve: (frame: RealtimeServerFrame) => void;
    reject: (err: Error) => void;
    timer: ReturnType<typeof setTimeout>;
  }> = [];

  private constructor(socket: WebSocket) {
    this.#socket = socket;
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

  static async connect(serverURL: string, bearerToken: string): Promise<RealtimeProtobufClient> {
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

    const client = new RealtimeProtobufClient(socket);
    client.send(
      new RealtimeClientFrame({
        frame: {
          case: 'hello',
          value: new RealtimeClientHello({ protocolVersion: 2, bearerToken })
        }
      })
    );
    await client.waitForFrame((frame) => frame.frame.case === 'hello');
    client.send(
      new RealtimeClientFrame({
        frame: { case: 'subscribeEvents', value: new RealtimeSubscribeEvents() }
      })
    );
    await client.waitForFrame((frame) => frame.frame.case === 'subscribed');
    return client;
  }

  close(): void {
    this.#socket.close();
    this.#rejectAll(new Error('realtime socket closed'));
  }

  send(frame: RealtimeClientFrame): void {
    this.#socket.send(frame.toBinary());
  }

  waitForEvent(
    predicate: (event: RealtimeEventEnvelope) => boolean
  ): Promise<RealtimeEventEnvelope> {
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
          const queued = this.#frames.map((frame) =>
            frame.frame.case === 'event'
              ? `event:${frame.frame.value.event.case ?? 'unknown'}`
              : (frame.frame.case ?? 'unknown')
          );
          reject(new Error(`timed out waiting for realtime frame; queued: ${queued.join(', ')}`));
        }, TIMEOUTS.REALTIME_EVENT)
      };
      this.#waiters.push(waiter);
    });
  }

  async #handleMessage(data: unknown): Promise<void> {
    const frame = RealtimeServerFrame.fromBinary(await websocketDataToBytes(data));
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
  test('materialises a cold room timeline through the projection stream', async ({
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
      await realtime.waitForFrame((frame) => frame.frame.case === 'caughtUp');
      realtime.send(
        new RealtimeClientFrame({
          frame: {
            case: 'hydrateRoom',
            value: new RealtimeHydrateRoom({ roomId })
          }
        })
      );

      const hydrated = await realtime.waitForFrame((frame) => {
        if (frame.frame.case !== 'projectionEvent') return false;
        return frame.frame.value.operations.some((operation) => {
          if (operation.operation.case !== 'roomTimelineReplace') return false;
          return (
            operation.operation.value.roomId === roomId &&
            operation.operation.value.page?.events.some((event) => event.id === messageId)
          );
        });
      });
      expect(hydrated.frame.case).toBe('projectionEvent');
      if (hydrated.frame.case !== 'projectionEvent') throw new Error('expected projection event');
      expect(
        hydrated.frame.value.operations.some(
          (operation) =>
            operation.operation.case === 'roomUpsert' &&
            operation.operation.value.room?.room?.id === roomId &&
            operation.operation.value.memberUserIds.includes(viewer.id)
        )
      ).toBe(true);
    } finally {
      realtime.close();
    }
  });

  test('delivers mention and DM display payloads over /api/realtime', async ({
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

      const mentionEvent = await realtime.waitForEvent(
        (event) => event.event.case === 'mentionNotification'
      );
      expect(mentionEvent.event.case).toBe('mentionNotification');
      expect(mentionEvent.event.value).toEqual(
        expect.objectContaining({
          actorDisplayName: mentionActorDisplayName,
          roomName: 'general'
        })
      );
      expect(mentionEvent.event.value.actorUserId).toBeTruthy();
      expect(mentionEvent.event.value.roomId).toBeTruthy();

      let dmSenderDisplayName = '';
      await withServerUser(browser!, serverURL, async ({ user, page: senderPage }) => {
        dmSenderDisplayName = user.displayName;
        const dmPage = new DMPage(senderPage);
        const roomPage = await dmPage.startConversation(viewer.login);
        await roomPage.sendMessage(`protobuf dm ${Date.now()}`);
      });

      const dmEvent = await realtime.waitForEvent(
        (event) => event.event.case === 'newDirectMessageNotification'
      );
      expect(dmEvent.event.case).toBe('newDirectMessageNotification');
      expect(dmEvent.event.value).toEqual(
        expect.objectContaining({
          senderDisplayName: dmSenderDisplayName,
          conversationName: dmSenderDisplayName
        })
      );
      expect(dmEvent.event.value.senderId).toBeTruthy();
      expect(dmEvent.event.value.roomId).toBeTruthy();
    } finally {
      realtime.close();
    }
  });
});
