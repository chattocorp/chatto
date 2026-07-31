import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { userEvent } from 'vitest/browser';
import { q } from '$lib/test-utils';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    createRoom: vi.fn(),
    joinRoom: vi.fn(),
    onroomcreated: vi.fn(),
    scopeCurrent: true
  }
}));

vi.mock('$lib/state/server/scope.svelte', () => ({
  useServerScope: () => ({
    serverId: 'origin',
    store: {},
    connection: {
      serverId: 'origin',
      connectBaseUrl: 'https://chat.example.test/api/connect',
      bearerToken: 'token',
      getAPI: (factory: (config: never) => unknown) => factory({} as never)
    },
    isCurrent: () => mocks.scopeCurrent
  })
}));

vi.mock('$lib/api-client/rooms', () => ({
  createRoomCommandAPI: () => ({
    createRoom: mocks.createRoom,
    joinRoom: mocks.joinRoom
  })
}));

async function fillNameAndSubmit(container: HTMLElement, name = 'general'): Promise<void> {
  await userEvent.type(q(container, '#room-name') as HTMLInputElement, name);
  (q(container, 'button[type="submit"]') as HTMLButtonElement).click();
}

import CreateRoom from './CreateRoom.svelte';

beforeEach(() => {
  vi.clearAllMocks();
  mocks.scopeCurrent = true;
  mocks.createRoom.mockResolvedValue({ id: 'room-1', name: 'general', description: '' });
  mocks.joinRoom.mockResolvedValue({ id: 'room-1', name: 'general', description: '' });
});

describe('CreateRoom', () => {
  it('creates a normal room through ConnectRPC and joins it', async () => {
    const { container } = render(CreateRoom, {
      groupId: 'group-1',
      onroomcreated: mocks.onroomcreated
    });

    await fillNameAndSubmit(container);

    await vi.waitFor(() => {
      expect(mocks.onroomcreated).toHaveBeenCalledWith('room-1');
    });
    expect(mocks.createRoom).toHaveBeenCalledWith({
      name: 'general',
      description: null,
      groupId: 'group-1',
      universal: false
    });
    expect(mocks.joinRoom).toHaveBeenCalledWith('room-1');
  });

  it('passes the universal flag to ConnectRPC', async () => {
    const { container } = render(CreateRoom, {
      groupId: 'group-1',
      onroomcreated: mocks.onroomcreated
    });

    (q(container, '#room-universal') as HTMLInputElement).click();
    await fillNameAndSubmit(container);

    await vi.waitFor(() => {
      expect(mocks.onroomcreated).toHaveBeenCalledWith('room-1');
    });
    expect(mocks.createRoom).toHaveBeenCalledWith({
      name: 'general',
      description: null,
      groupId: 'group-1',
      universal: true
    });
  });

  it('normalizes Unicode room names before creation', async () => {
    const { container } = render(CreateRoom, {
      groupId: 'group-1',
      onroomcreated: mocks.onroomcreated
    });

    await fillNameAndSubmit(container, 'Ku\u0308che');

    await vi.waitFor(() => {
      expect(mocks.createRoom).toHaveBeenCalledWith({
        name: 'Küche',
        description: null,
        groupId: 'group-1',
        universal: false
      });
    });
  });

  it('does not publish a completed room after its server scope is replaced', async () => {
    let finishJoining: (() => void) | undefined;
    mocks.joinRoom.mockImplementation(
      () =>
        new Promise((resolve) => {
          finishJoining = () => resolve({ id: 'room-1', name: 'general', description: '' });
        })
    );
    const { container } = render(CreateRoom, {
      groupId: 'group-1',
      onroomcreated: mocks.onroomcreated
    });

    await fillNameAndSubmit(container);
    await vi.waitFor(() => expect(mocks.joinRoom).toHaveBeenCalledWith('room-1'));

    mocks.scopeCurrent = false;
    finishJoining?.();

    await vi.waitFor(() => {
      expect((q(container, 'button[type="submit"]') as HTMLButtonElement).disabled).toBe(false);
    });
    expect(mocks.onroomcreated).not.toHaveBeenCalled();
  });
});
