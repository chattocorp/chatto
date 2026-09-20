import '../../../app.css';
import { afterEach, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import { serverRegistry } from '$lib/state/server/registry.svelte';
import { RoomWithViewerState } from '@chatto/api-types/api/v1/room_directory_pb';
import VoiceCallPanelStoryHarness from './VoiceCallPanelStoryHarness.svelte';
import { serverIdToSegment } from '$lib/navigation';

const { goto } = vi.hoisted(() => ({ goto: vi.fn() }));
vi.mock('$app/navigation', async (original) => ({
  ...(await original<typeof import('$app/navigation')>()),
  goto
}));

afterEach(() => vi.restoreAllMocks());

it('updates network warnings independently of microphone activity and clears them on recovery', async () => {
  const screen = render(VoiceCallPanelStoryHarness, {
    props: { layout: 'sidebar', scenario: 'voice' }
  });
  const call = serverRegistry.getStore(serverRegistry.originServer!.id).voiceCall;
  await expect
    .element(screen.getByRole('button', { name: 'Poor connection', exact: true }))
    .toBeInTheDocument();
  flushSync(() => {
    call.participants = call.participants.map((p) => ({
      ...p,
      connectionQuality: p.isLocal ? 'lost' : 'excellent'
    }));
  });
  await expect
    .element(screen.getByRole('button', { name: 'Poor connection', exact: true }))
    .not.toBeInTheDocument();
  await screen.getByRole('button', { name: 'Connection lost', exact: true }).click();
  await expect.element(screen.getByRole('dialog', { name: 'Connection lost' })).toBeInTheDocument();
  flushSync(() => {
    call.participants = call.participants.map((p) => ({ ...p, connectionQuality: 'excellent' }));
  });
  await expect.element(screen.getByTestId('call-connection-quality')).not.toBeInTheDocument();
  await expect
    .element(screen.getByRole('dialog', { name: 'Connection lost' }))
    .not.toBeInTheDocument();
});

it('opens voice preferences from the toolbar gear', async () => {
  const screen = render(VoiceCallPanelStoryHarness, {
    props: { layout: 'sidebar', scenario: 'voice' }
  });
  await screen.getByTestId('call-device-menu-button').click();
  expect(goto).toHaveBeenCalledWith(
    `/chat/${serverIdToSegment(serverRegistry.originServer!.id)}/settings/voice`
  );
});

it('shows the microphone warning only on the local participant card', async () => {
  const screen = render(VoiceCallPanelStoryHarness, {
    props: { layout: 'sidebar', scenario: 'voice' }
  });
  const call = serverRegistry.getStore(serverRegistry.originServer!.id).voiceCall;
  flushSync(() => {
    call.microphoneSilent = true;
  });
  await expect.element(screen.getByTestId('microphone-silence-hint')).toBeInTheDocument();
  expect(
    screen.container.querySelector('[title="Alice"] [data-testid="microphone-silence-hint"]')
  ).not.toBeNull();
  expect(
    screen.container.querySelector('[title="Bob"] [data-testid="microphone-silence-hint"]')
  ).toBeNull();
  flushSync(() => {
    call.microphoneSilent = false;
  });
  await expect.element(screen.getByTestId('microphone-silence-hint')).not.toBeInTheDocument();
});

it('removes voice activity and participant controls when a call becomes observed', async () => {
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
  await expect
    .poll(() => bob.querySelector('[data-testid="voice-activity"]')?.getAttribute('data-active'))
    .toBe('true');

  flushSync(() => {
    store.voiceCall.connected = false;
    store.voiceCall.roomId = null;
  });

  await expect.element(screen.getByTestId('call-observer-panel')).toBeInTheDocument();
  await expect.element(screen.getByTestId('call-join-button')).toBeInTheDocument();
  expect(screen.container.querySelector('[title="Bob"]')).toBe(bob);
  expect(bob.querySelector('[data-testid="voice-activity"]')).toBeNull();
  expect(screen.container.querySelector('[data-testid="call-feed-local-mute-button"]')).toBeNull();
});

it('settles voice activity when the participant mutes their microphone', async () => {
  const screen = render(VoiceCallPanelStoryHarness, {
    props: { layout: 'sidebar', scenario: 'voice' }
  });
  await expect.element(screen.getByTestId('call-participant-panel')).toBeInTheDocument();
  const call = serverRegistry.getStore(serverRegistry.originServer!.id).voiceCall;
  vi.spyOn(call, 'getAudioLevel').mockReturnValue({ isSpeaking: true, audioLevel: 0.5 });
  const canvas = screen.container.querySelector<HTMLCanvasElement>(
    '[title="Bob"] [data-testid="voice-activity"]'
  )!;
  await expect.poll(() => canvas.dataset.active).toBe('true');
  flushSync(() => {
    call.participants = call.participants.map((participant) => ({ ...participant, isMuted: true }));
  });
  await expect.poll(() => canvas.dataset.active, { timeout: 4000 }).toBe('false');
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
  const selfCard = screen.container.querySelector<HTMLElement>('[title="Alice"]')!;
  expect(selfCard.querySelector('[data-testid="call-feed-local-mute-button"]')).toBeNull();
  expect(selfCard.querySelector('[data-testid="call-muted-indicator"]')).not.toBeNull();
  flushSync(() => {
    store.voiceCall.connected = false;
  });
  vi.spyOn(store.activeCallRooms, 'has').mockReturnValue(false);
  await expect.element(screen.getByTestId('call-join-button')).toBeDisabled();
});

it('includes volume controls in the remote user context menu', async () => {
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
  expect(document.querySelector('[data-testid="copy-user-id"]')).not.toBeNull();
  const input = document.querySelector<HTMLInputElement>('input[type="range"]')!;
  expect(document.querySelectorAll('input[type="range"]')).toHaveLength(1);
  expect(input.max).toBe('200');
  input.value = '175';
  input.dispatchEvent(new Event('input', { bubbles: true }));
  expect(change).toHaveBeenCalledWith('bob', 'voiceVolume', 175);
  expect(
    screen.container.querySelector('[title="Alice"] [data-testid="call-participant-menu-button"]')
  ).not.toBeNull();
  flushSync(() => {
    store.voiceCall.connected = false;
  });
  expect(document.querySelector('input[type="range"]')).toBeNull();
});

it('opens the user menu and volume controls by right-clicking a media card', async () => {
  const screen = render(VoiceCallPanelStoryHarness, {
    props: { layout: 'stage', scenario: 'screen' }
  });
  await expect.element(screen.getByTestId('call-featured-stage-card')).toBeInTheDocument();
  const card = screen.container.querySelector('[data-testid="call-featured-stage-card"]')!;
  const event = new MouseEvent('contextmenu', {
    bubbles: true,
    cancelable: true,
    clientX: 120,
    clientY: 80
  });
  card.dispatchEvent(event);
  expect(event.defaultPrevented).toBe(true);
  await expect.poll(() => document.querySelector('[data-testid="copy-user-id"]')).not.toBeNull();
  expect(document.querySelectorAll('input[type="range"]')).toHaveLength(1);
  const call = serverRegistry.getStore(serverRegistry.originServer!.id).voiceCall;
  const change = vi.spyOn(call, 'setParticipantVolume');
  const input = document.querySelector<HTMLInputElement>('input[type="range"]')!;
  input.value = '60';
  input.dispatchEvent(new Event('input', { bubbles: true }));
  expect(change).toHaveBeenCalledWith('dana', 'streamVolume', 60);
});

it.each(['sidebar', 'stage'] as const)(
  'uses screen audio independently in the %s screen tile',
  async (layout) => {
    const screen = render(VoiceCallPanelStoryHarness, {
      props: { layout, scenario: 'screen' }
    });
    await expect.element(screen.getByTestId('call-participant-panel')).toBeInTheDocument();
    const call = serverRegistry.getStore(serverRegistry.originServer!.id).voiceCall;
    const mic = vi
      .spyOn(call, 'getAudioLevel')
      .mockReturnValue({ isSpeaking: true, audioLevel: 0.5 });
    const stream = vi.spyOn(call, 'getScreenShareAudioLevel').mockReturnValue(0);
    const tile = screen.container.querySelector(
      layout === 'stage'
        ? '[data-testid="call-featured-stage-card"]'
        : '[data-testid="call-screen-share-card"]'
    )!;
    const canvas = tile.querySelector<HTMLCanvasElement>('[data-testid="voice-activity"]')!;
    await expect.poll(() => stream.mock.calls.length).toBeGreaterThan(1);
    expect(canvas.dataset.active).toBe('false');
    mic.mockReturnValue({ isSpeaking: false, audioLevel: 0 });
    stream.mockReturnValue(0.5);
    await expect.poll(() => canvas.dataset.active).toBe('true');
    stream.mockReturnValue(0);
    await expect.poll(() => canvas.dataset.active, { timeout: 3000 }).toBe('false');
  }
);

it('keeps the current user menu free of listener volume controls', async () => {
  const screen = render(VoiceCallPanelStoryHarness, {
    props: { layout: 'sidebar', scenario: 'voice' }
  });
  await expect.element(screen.getByTestId('call-participant-panel')).toBeInTheDocument();
  screen.container
    .querySelector<HTMLButtonElement>(
      '[title="Alice"] [data-testid="call-participant-menu-button"]'
    )!
    .click();
  await expect.poll(() => document.querySelector('[data-testid="copy-user-id"]')).not.toBeNull();
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

it('keeps voice cards equal in height with compact direct mute controls', async () => {
  const screen = render(VoiceCallPanelStoryHarness, {
    props: { layout: 'sidebar', scenario: 'voice' }
  });
  await expect.element(screen.getByTestId('call-participant-panel')).toBeInTheDocument();
  const store = serverRegistry.getStore(serverRegistry.originServer!.id);
  const muteRemote = vi.spyOn(store.voiceCall, 'toggleParticipantLocalMute');
  const muteSelf = vi.spyOn(store.voiceCall, 'toggleMute').mockResolvedValue();
  const cards = [
    ...screen.container.querySelectorAll<HTMLElement>('[data-testid="call-participant-card"]')
  ];
  const heights = cards.map((card) => card.getBoundingClientRect().height);
  expect(cards).toHaveLength(3);
  expect(heights[0]).toBe(3 * parseFloat(getComputedStyle(document.documentElement).fontSize));
  expect(heights.every((height) => height === heights[0])).toBe(true);
  for (const card of cards) {
    const button = card.querySelector<HTMLElement>('[data-testid="call-feed-local-mute-button"]')!;
    expect(button.getBoundingClientRect().height).toBeLessThanOrEqual(28);
  }
  const bob = screen.container.querySelector<HTMLElement>('[title="Bob"]')!;
  expect(bob.textContent).toContain('@bob');
  expect(getComputedStyle(bob).borderTopWidth).toBe('0px');
  bob.querySelector<HTMLButtonElement>('[data-testid="call-feed-local-mute-button"]')!.click();
  expect(muteRemote).toHaveBeenCalledWith('bob');
  const alice = screen.container.querySelector<HTMLElement>('[title="Alice"]')!;
  const selfMuteButton = alice.querySelector<HTMLButtonElement>(
    '[data-testid="call-feed-local-mute-button"]'
  )!;
  expect(selfMuteButton.getAttribute('aria-label')).toBe('Mute');
  expect(
    selfMuteButton.querySelector('.iconify')?.classList.contains('icon-[uil--microphone]')
  ).toBe(true);
  selfMuteButton.click();
  expect(muteSelf).toHaveBeenCalledOnce();
  flushSync(() => {
    store.voiceCall.isMuted = true;
    store.voiceCall.participants = store.voiceCall.participants.map((participant) => ({
      ...participant,
      isMuted: true
    }));
  });
  expect(selfMuteButton.getAttribute('aria-label')).toBe('Unmute');
  expect(
    selfMuteButton.querySelector('.iconify')?.classList.contains('icon-[uil--microphone-slash]')
  ).toBe(true);
  expect(selfMuteButton.querySelector('.iconify')?.classList.contains('text-danger')).toBe(true);
  expect(alice.querySelector('[data-testid="call-muted-indicator"]')).toBeNull();
  expect(bob.querySelector('[data-testid="call-muted-indicator"]')).not.toBeNull();
  expect(
    bob
      .querySelector('[data-testid="call-feed-local-mute-button"] .iconify')
      ?.classList.contains('icon-[uil--volume-mute]')
  ).toBe(true);
  selfMuteButton.click();
  expect(muteSelf).toHaveBeenCalledTimes(2);
});

it('keeps voice columns equal below a single screen share and stacks in a narrow pane', async () => {
  const screen = render(VoiceCallPanelStoryHarness, {
    props: { layout: 'sidebar', scenario: 'screen-voice' }
  });
  screen.container.style.width = '500px';
  await expect.element(screen.getByTestId('call-participant-panel')).toBeInTheDocument();
  const cards = [
    ...screen.container.querySelectorAll<HTMLElement>('[data-testid="call-participant-card"]')
  ];
  const list = screen.container.querySelector<HTMLElement>(
    '[data-testid="call-participants-list"]'
  )!;
  const share = list.querySelector<HTMLElement>('[data-call-media-card]')!;
  await expect
    .poll(() =>
      Math.abs(cards[0].getBoundingClientRect().width - cards[1].getBoundingClientRect().width)
    )
    .toBeLessThan(1);
  expect(cards[0].getBoundingClientRect().top).toBe(cards[1].getBoundingClientRect().top);
  expect(share.getBoundingClientRect().width).toBe(list.getBoundingClientRect().width);
  expect(cards[0].getBoundingClientRect().width).toBeLessThan(list.getBoundingClientRect().width);

  screen.container.style.width = '280px';
  await expect
    .poll(() => cards[0].getBoundingClientRect().width)
    .toBe(list.getBoundingClientRect().width);
  expect(cards[1].getBoundingClientRect().top).toBeGreaterThan(
    cards[0].getBoundingClientRect().top
  );
  expect(share.getBoundingClientRect().width).toBe(list.getBoundingClientRect().width);
});
