import { flushSync } from 'svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import type { ServerStateStore } from '@chatto/client/server/store';
import IncomingCallAlert from './IncomingCallAlert.svelte';

const mocks = vi.hoisted(() => ({
  goto: vi.fn(),
  markRead: vi.fn(async () => {})
}));
vi.mock('$app/navigation', async (original) => ({
  ...(await original<typeof import('$app/navigation')>()),
  goto: mocks.goto
}));

vi.mock('$lib/state/server/serverUi', () => ({ serverUi: (store: unknown) => store }));

function fixture() {
  const state = $state({ phase: 'ready', callId: 'original', joined: false });
  const store = {
    projection: { rooms: new Map() },
    serverId: 'server',
    isAuthenticated: true,
    accountId: 'viewer',
    viewerId: 'viewer',
    serverInfo: { livekitUrl: 'wss://example.invalid', name: 'Server' },
    realtimeSync: state,
    activeCallRooms: {
      getCallId: () => state.callId,
      getParticipantCallPresence: () => (state.joined ? 'voice' : null)
    },
    navigation: { rooms: [{ id: 'room', name: 'Room' }] },
    notifications: {
      occurrences: [{ id: 'row', unread: true, actor: { displayName: 'Taylor' } }],
      markOccurrenceRead: mocks.markRead
    },
    voiceCall: {
      permissionsFor: () => ({ join: true })
    }
  } as unknown as ServerStateStore;
  const close = vi.fn();
  const view = render(IncomingCallAlert, {
    store,
    roomId: 'room',
    callId: 'original',
    occurrenceId: 'row',
    onclose: close
  });
  return { view, state, close };
}

describe('incoming call alert', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('opens the room and dismisses the occurrence', async () => {
    const { view, close } = fixture();
    expect(view.container.textContent).toContain('Taylor started a call');
    await view.getByRole('button', { name: 'Open room' }).click();
    expect(mocks.goto).toHaveBeenCalledWith('/chat/-/room');
    expect(close).toHaveBeenCalledOnce();
    expect(mocks.markRead).toHaveBeenCalledWith('row');
  });

  it('dismisses the occurrence without navigating', async () => {
    const { view, close } = fixture();
    await view.getByRole('button', { name: 'Dismiss notification' }).click();
    expect(close).toHaveBeenCalled();
    expect(mocks.markRead).toHaveBeenCalledWith('row');
    expect(mocks.goto).not.toHaveBeenCalled();
  });

  it('closes the live alert when the recipient joins elsewhere', () => {
    const { view, close, state } = fixture();
    state.joined = true;
    flushSync();
    expect(close).toHaveBeenCalled();
    expect(view.container.querySelector('button')).toBeNull();
  });

  it('closes a replaced call alert after the current snapshot arrives', () => {
    const { view, state, close } = fixture();
    state.phase = 'hydrating';
    state.callId = 'replacement';
    flushSync();
    expect(close).not.toHaveBeenCalled();
    expect(view.container.querySelector('button')).toBeNull();
    state.phase = 'ready';
    flushSync();
    expect(close).toHaveBeenCalled();
    expect(mocks.goto).not.toHaveBeenCalled();
  });
});
