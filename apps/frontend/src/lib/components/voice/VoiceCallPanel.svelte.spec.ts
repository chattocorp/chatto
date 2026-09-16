import { afterEach, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import { serverRegistry } from '$lib/state/server/registry.svelte';
import { RoomWithViewerState } from '@chatto/api-types/api/v1/room_directory_pb';
import VoiceCallPanelStoryHarness from './VoiceCallPanelStoryHarness.svelte';

afterEach(() => vi.restoreAllMocks());

it('clears speaking styles and participant controls when a call becomes observed', async () => {
  const screen = render(VoiceCallPanelStoryHarness, {
    props: { layout: 'sidebar', scenario: 'voice' }
  });
  await expect.element(screen.getByTestId('call-participant-panel')).toBeInTheDocument();
  const store = serverRegistry.getStore(serverRegistry.originServer!.id);
  vi.spyOn(store.voiceCall, 'getAudioLevel').mockReturnValue({ isSpeaking: true, audioLevel: 0.5 });
  vi.spyOn(store.activeCallRooms, 'has').mockReturnValue(true);
  vi.spyOn(store.activeCallRooms, 'getParticipants').mockReturnValue([
    { userId: 'bob', login: 'bob', displayName: 'Bob', avatarUrl: null, isBot: false }
  ]);
  const bob = screen.container.querySelector<HTMLElement>('[title="Bob"]')!;
  await expect.poll(() => bob.style.getPropertyValue('--call-speaking-ring-opacity')).not.toBe('0');
  await expect.poll(() => bob.dataset.callSpeaking).toBe('true');

  flushSync(() => {
    store.voiceCall.connected = false;
    store.voiceCall.roomId = null;
  });

  await expect.element(screen.getByTestId('call-observer-panel')).toBeInTheDocument();
  await expect.element(screen.getByTestId('call-join-button')).toBeInTheDocument();
  expect(screen.container.querySelector('[title="Bob"]')).toBe(bob);
  expect(bob.style.getPropertyValue('--call-speaking-ring-opacity')).toBe('');
  expect(bob.style.getPropertyValue('--call-speaking-ring-strength')).toBe('');
  expect(bob.dataset.callSpeaking).toBeUndefined();
  expect(bob.hasAttribute('data-speaking-ring')).toBe(false);
  expect(screen.container.querySelector('[data-testid="call-feed-local-mute-button"]')).toBeNull();
});

it('gates entry and media controls from the current room permissions', async () => {
  const screen = render(VoiceCallPanelStoryHarness, {
    props: { layout: 'sidebar', scenario: 'voice' }
  });
  await expect.element(screen.getByTestId('call-participant-panel')).toBeInTheDocument();
  const store = serverRegistry.getStore(serverRegistry.originServer!.id);
  const roomId = store.voiceCall.roomId!;
  const room = store.projection.rooms.get(roomId)!;
  flushSync(() => {
    store.voiceCall.isMuted = true;
    store.projection.rooms.set(
      roomId,
      new RoomWithViewerState({
        room: room.room,
        viewerState: {
          isMember: true,
          permissions: [{ permission: 'call.join', granted: true }]
        }
      })
    );
  });
  await expect.element(screen.getByTestId('call-mute-toggle')).toBeDisabled();
  await expect.element(screen.getByTestId('call-camera-toggle')).toBeDisabled();
  await expect.element(screen.getByTestId('call-screen-share-toggle')).toBeDisabled();
  await expect.element(screen.getByTestId('call-leave-button')).toBeEnabled();
  flushSync(() => {
    store.voiceCall.connected = false;
  });
  vi.spyOn(store.activeCallRooms, 'has').mockReturnValue(false);
  await expect.element(screen.getByTestId('call-join-button')).toBeDisabled();
});

it('opens volume controls from the remote card menu without opening a profile', async () => {
  const screen = render(VoiceCallPanelStoryHarness, {
    props: { layout: 'sidebar', scenario: 'voice' }
  });
  await expect.element(screen.getByTestId('call-participant-panel')).toBeInTheDocument();
  const store = serverRegistry.getStore(serverRegistry.originServer!.id);
  const change = vi.spyOn(store.voiceCall, 'setParticipantVolume');
  const bob = screen.container.querySelector<HTMLElement>('[title="Bob"]')!;
  expect(bob.querySelector('input[type="range"]')).toBeNull();
  bob.querySelector<HTMLButtonElement>('[data-testid="call-participant-menu-button"]')!.click();
  await expect.poll(() => document.querySelector('input[type="range"]')).not.toBeNull();
  const input = document.querySelector<HTMLInputElement>('input[type="range"]')!;
  expect(input.max).toBe('200');
  input.value = '175';
  input.dispatchEvent(new Event('input', { bubbles: true }));
  expect(change).toHaveBeenCalledWith('bob', 'voiceVolume', 175);
  expect(
    screen.container.querySelector('[title="Alice"] [data-testid="call-participant-menu-button"]')
  ).toBeNull();
  flushSync(() => {
    store.voiceCall.connected = false;
  });
  expect(document.querySelector('input[type="range"]')).toBeNull();
});

it('places the overflow menu in the screen-share card header', async () => {
  const screen = render(VoiceCallPanelStoryHarness, {
    props: { layout: 'stage', scenario: 'screen' }
  });
  await expect.element(screen.getByTestId('call-featured-stage-card')).toBeInTheDocument();
  const card = screen.container.querySelector('[data-testid="call-featured-stage-card"]')!;
  expect(card.querySelector('[data-testid="call-participant-menu-button"]')).not.toBeNull();
  expect(card.querySelector('input[type="range"]')).toBeNull();
});
