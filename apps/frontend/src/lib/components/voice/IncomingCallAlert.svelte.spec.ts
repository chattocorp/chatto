import { flushSync } from 'svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import type { ServerStateStore } from '@chatto/client/server/store';
import { toast } from '$lib/ui/toast';
import IncomingCallAlert from './IncomingCallAlert.svelte';

const mocks = vi.hoisted(() => ({
  goto: vi.fn(),
  join: vi.fn(),
  matchesActiveCall: vi.fn(() => true),
  markRead: vi.fn(async () => {}),
  panel: vi.fn(),
  ended: vi.fn()
}));
vi.mock('$app/navigation', async (original) => ({
  ...(await original<typeof import('$app/navigation')>()),
  goto: mocks.goto
}));

vi.mock('$lib/state/appUi.svelte', async (original) => ({
  ...(await original<typeof import('$lib/state/appUi.svelte')>()),
  getAppUiState: () => ({ requestRoomSidebarPanel: mocks.panel }),
  getRoomSidebarPresentation: () => 'desktop'
}));
vi.mock('$lib/state/server/serverUi', () => ({ serverUi: (store: unknown) => store }));

function fixture(fromLink = false) {
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
      permissionsFor: () => ({ join: true }),
      join: mocks.join,
      matchesActiveCall: mocks.matchesActiveCall
    }
  } as unknown as ServerStateStore;
  const close = vi.fn();
  const view = render(IncomingCallAlert, {
    store,
    roomId: 'room',
    callId: 'original',
    occurrenceId: 'row',
    fromLink,
    onclose: close
  });
  return { view, state, close };
}

describe('incoming call alert', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.spyOn(toast, 'info').mockImplementation(mocks.ended);
  });

  it('joins only the call whose alert the member opened', async () => {
    const { view } = fixture();
    expect(view.container.textContent).toContain('Taylor started a call');
    await view.getByRole('button', { name: 'Join call' }).click();
    expect(mocks.join).toHaveBeenCalledWith('wss://example.invalid', 'room', 'original');
    await vi.waitFor(() => expect(mocks.goto).toHaveBeenCalled());
  });

  it('stays on the current page when an in-flight join is cancelled', async () => {
    const { view } = fixture();
    mocks.matchesActiveCall.mockReturnValueOnce(false);
    const error = vi.spyOn(toast, 'error');
    await view.getByRole('button', { name: 'Join call' }).click();
    await vi.waitFor(() =>
      expect(mocks.matchesActiveCall).toHaveBeenCalledWith('room', 'original')
    );
    expect(mocks.goto).not.toHaveBeenCalled();
    expect(error).not.toHaveBeenCalled();
  });

  it('dismisses the occurrence without joining', async () => {
    const { view, close } = fixture();
    await view.getByRole('button', { name: 'Dismiss notification' }).click();
    expect(close).toHaveBeenCalled();
    expect(mocks.markRead).toHaveBeenCalledWith('row');
    expect(mocks.join).not.toHaveBeenCalled();
  });

  it('closes the live alert when the recipient joins elsewhere', () => {
    const { view, close, state } = fixture();
    state.joined = true;
    flushSync();
    expect(close).toHaveBeenCalled();
    expect(view.container.querySelector('button')).toBeNull();
    expect(mocks.ended).not.toHaveBeenCalled();
  });

  it('waits for the current snapshot before reporting a stale link', () => {
    const { view, state, close } = fixture(true);
    state.phase = 'hydrating';
    state.callId = 'replacement';
    flushSync();
    expect(close).not.toHaveBeenCalled();
    expect(view.container.querySelector('button')).toBeNull();
    state.phase = 'ready';
    flushSync();
    expect(close).toHaveBeenCalled();
    expect(mocks.ended).toHaveBeenCalledWith('The active call has ended');
    expect(mocks.join).not.toHaveBeenCalled();
  });
});
