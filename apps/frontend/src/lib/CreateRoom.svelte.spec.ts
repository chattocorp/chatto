import { beforeEach, describe, expect, it, vi } from 'vitest';
import { flushSync } from 'svelte';
import { render } from 'vitest-browser-svelte';
import { q } from '$lib/test-utils';
import { RoomThreadingMode } from '$lib/roomThreading';

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

function fillName(container: HTMLElement, name: string): void {
  const input = q(container, '#room-name') as HTMLInputElement;
  input.dispatchEvent(new KeyboardEvent('keydown', { bubbles: true, key: 'a' }));
  input.value = name;
  input.dispatchEvent(new Event('input', { bubbles: true }));
  flushSync();
}

async function fillNameAndSubmit(container: HTMLElement, name = 'general'): Promise<void> {
  fillName(container, name);
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
      universal: false,
      threadingMode: RoomThreadingMode.ENABLED
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
      universal: true,
      threadingMode: RoomThreadingMode.ENABLED
    });
  });

  it('accepts spaces, punctuation, emoji, and normalizes Unicode before creation', async () => {
    const { container } = render(CreateRoom, {
      groupId: 'group-1',
      onroomcreated: mocks.onroomcreated
    });

    await fillNameAndSubmit(container, '  Team chat 💬 / Ku\u0308che!  ');

    await vi.waitFor(() => {
      expect(mocks.createRoom).toHaveBeenCalledWith({
        name: 'Team chat 💬 / Küche!',
        description: null,
        groupId: 'group-1',
        universal: false,
        threadingMode: RoomThreadingMode.ENABLED
      });
    });
  });

  it('passes the selected threading mode to ConnectRPC', async () => {
    const { container } = render(CreateRoom, {
      groupId: 'group-1',
      onroomcreated: mocks.onroomcreated
    });

    const choices = Array.from(container.querySelectorAll<HTMLButtonElement>('[role="radio"]'));
    expect(choices).toHaveLength(4);
    expect(choices.map((choice) => choice.textContent?.trim())).toEqual([
      expect.stringContaining('Required'),
      expect.stringContaining('Encouraged'),
      expect.stringContaining('Enabled'),
      expect.stringContaining('Disabled')
    ]);
    expect(choices[2]).toHaveAttribute('aria-checked', 'true');
    choices[0].click();
    await fillNameAndSubmit(container);

    await vi.waitFor(() => {
      expect(mocks.createRoom).toHaveBeenCalledWith(
        expect.objectContaining({ threadingMode: RoomThreadingMode.REQUIRED })
      );
    });
  });

  it.each([
    ['an invisible-only name', '\u200d\u2060', 'Room name is required'],
    ['a name containing a line separator', 'Team\u2028chat', 'control characters'],
    ['a name longer than 30 code points', '𐐀'.repeat(31), '30 characters']
  ])('rejects %s locally', async (_description, name, errorText) => {
    const { container } = render(CreateRoom, {
      groupId: 'group-1',
      onroomcreated: mocks.onroomcreated
    });

    fillName(container, name);
    const submit = q(container, 'button[type="submit"]') as HTMLButtonElement;

    expect(submit.disabled).toBe(true);
    expect(container.textContent).toContain(errorText);
    expect(mocks.createRoom).not.toHaveBeenCalled();
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
