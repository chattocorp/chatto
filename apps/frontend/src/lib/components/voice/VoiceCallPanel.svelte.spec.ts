import { afterEach, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import { serverRegistry } from '$lib/state/server/registry.svelte';
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
