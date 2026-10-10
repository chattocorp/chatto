import '../../../app.css';
import { afterEach, expect, it, vi } from 'vitest';
import { flushSync } from 'svelte';
import { serverRegistry } from '$lib/client';
import { serverUi } from '$lib/state/server/serverUi';
import { RoomWithViewerState } from '@chatto/api-types/api/v1/room_directory_pb';
import { renderCallPanelHarness } from './renderCallPanelHarness';
import { serverIdToSegment } from '$lib/navigation';

const { goto } = vi.hoisted(() => ({ goto: vi.fn() }));
vi.mock('$app/navigation', async (original) => ({
  ...(await original<typeof import('$app/navigation')>()),
  goto
}));

afterEach(() => vi.restoreAllMocks());

it.each([319, 320])('uses card width for overflow at %spx in a wide window', async (width) => {
  const screen = renderCallPanelHarness({ layout: 'sidebar', scenario: 'voice' });
  await expect.element(screen.getByTestId('call-participant-panel')).toBeInTheDocument();
  const card = screen.container.querySelector<HTMLElement>('[title="Alice"]')!;
  card.style.width = `${width}px`;
  await expect
    .poll(() => card.querySelector('[data-testid="call-feed-local-mute-button"]') !== null)
    .toBe(width === 320);
  expect(window.innerWidth).toBeGreaterThan(width);
  const menuButton = card.querySelector<HTMLButtonElement>(
    '[data-testid="call-participant-menu-button"]'
  )!;
  expect(menuButton.getAttribute('aria-label')).toBe('Actions for Alice');
  menuButton.click();
  await expect.poll(() => document.querySelector('[data-testid="copy-user-id"]')).not.toBeNull();
  expect(
    document.querySelector('[popover] [data-testid="call-feed-local-mute-button"]') !== null
  ).toBe(width < 320);
});

it('keeps mute toggles live in compact menus and keeps mute status visible', async () => {
  const screen = renderCallPanelHarness({
    layout: 'sidebar',
    scenario: 'voice',
    initiallyMuted: true
  });
  screen.container.style.width = '280px';
  await expect.element(screen.getByTestId('call-participant-panel')).toBeInTheDocument();
  const call = serverUi(serverRegistry.getStore(serverRegistry.originServer!.id)).voiceCall;
  const muteSelf = vi.spyOn(call, 'toggleMute').mockImplementation(async () => {
    call.isMuted = !call.isMuted;
  });
  const muteRemote = vi.spyOn(call, 'toggleParticipantLocalMute').mockImplementation((identity) => {
    call.participants = call.participants.map((participant) =>
      participant.identity === identity
        ? { ...participant, isLocallyMuted: !participant.isLocallyMuted }
        : participant
    );
  });
  const alice = screen.container.querySelector<HTMLElement>('[title="Alice"]')!;
  const bob = screen.container.querySelector<HTMLElement>('[title="Bob"]')!;
  await expect
    .poll(() => alice.querySelector('[data-testid="call-feed-local-mute-button"]'))
    .toBeNull();
  expect(alice.querySelector('[data-testid="call-muted-indicator"]')).not.toBeNull();
  expect(bob.querySelector('[data-testid="call-locally-muted-indicator"]')).not.toBeNull();
  alice.querySelector<HTMLButtonElement>('[data-testid="call-participant-menu-button"]')!.click();
  await expect
    .poll(() => document.querySelector('[popover] [data-testid="call-feed-local-mute-button"]'))
    .not.toBeNull();
  const toggle = document.querySelector<HTMLButtonElement>(
    '[popover] [data-testid="call-feed-local-mute-button"]'
  )!;
  expect(toggle.getAttribute('role')).toBeNull();
  expect(toggle.getAttribute('aria-pressed')).toBe('true');
  toggle.click();
  await expect.poll(() => toggle.getAttribute('aria-pressed')).toBe('false');
  expect(toggle.textContent?.trim()).toBe('Mute');
  expect(muteSelf).toHaveBeenCalledOnce();
  expect(alice.querySelector('[data-testid="call-muted-indicator"]')).toBeNull();
  expect(document.querySelector('[data-testid="copy-user-id"]')).not.toBeNull();
  bob.querySelector<HTMLButtonElement>('[data-testid="call-participant-menu-button"]')!.click();
  await expect
    .poll(() =>
      document
        .querySelector<HTMLButtonElement>('[popover] [data-testid="call-feed-local-mute-button"]')
        ?.textContent?.trim()
    )
    .toBe('Mute locally');
  document
    .querySelector<HTMLButtonElement>('[popover] [data-testid="call-feed-local-mute-button"]')!
    .click();
  await expect
    .poll(() => bob.querySelector('[data-testid="call-locally-muted-indicator"]'))
    .toBeNull();
  expect(muteRemote).toHaveBeenCalledWith('bob');
  expect(document.querySelector('input[type="range"]')).not.toBeNull();
});

it('dismisses overflow on resizing or removal and restores keyboard focus', async () => {
  const screen = renderCallPanelHarness({ layout: 'sidebar', scenario: 'voice' });
  await expect.element(screen.getByTestId('call-participant-panel')).toBeInTheDocument();
  const card = screen.container.querySelector<HTMLElement>('[title="Bob"]')!;
  const trigger = card.querySelector<HTMLButtonElement>(
    '[data-testid="call-participant-menu-button"]'
  )!;
  card.style.width = '319px';
  await expect
    .poll(() => card.querySelector('[data-testid="call-feed-local-mute-button"]'))
    .toBeNull();
  trigger.click();
  await expect
    .poll(() => document.querySelector('[popover] [data-testid="call-feed-local-mute-button"]'))
    .not.toBeNull();
  const toggle = document.querySelector<HTMLButtonElement>(
    '[popover] [data-testid="call-feed-local-mute-button"]'
  )!;
  toggle.focus();
  window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
  await expect.poll(() => document.activeElement).toBe(trigger);
  trigger.click();
  await expect.poll(() => document.querySelector('[data-testid="copy-user-id"]')).not.toBeNull();
  card.style.width = '320px';
  await expect.poll(() => document.querySelector('[data-testid="copy-user-id"]')).toBeNull();
  expect(document.activeElement).toBe(trigger);
  const inline = card.querySelector<HTMLButtonElement>(
    '[data-testid="call-feed-local-mute-button"]'
  )!;
  inline.focus();
  card.style.width = '319px';
  await expect.poll(() => document.activeElement).toBe(trigger);
  trigger.click();
  await expect.poll(() => document.querySelector('[data-testid="copy-user-id"]')).not.toBeNull();
  const call = serverUi(serverRegistry.getStore(serverRegistry.originServer!.id)).voiceCall;
  flushSync(() => {
    call.participants = call.participants.filter((participant) => participant.identity !== 'bob');
  });
  await expect.poll(() => document.querySelector('[data-testid="copy-user-id"]')).toBeNull();
  await expect
    .poll(() => document.activeElement?.getAttribute('data-testid'))
    .toBe('call-participant-menu-button');
  expect(document.activeElement?.isConnected).toBe(true);
});

it('puts compact call actions in a touch sheet without opening a duplicate menu', async () => {
  const screen = renderCallPanelHarness({ layout: 'sidebar', scenario: 'voice' });
  screen.container.style.width = '280px';
  await expect.element(screen.getByTestId('call-participant-panel')).toBeInTheDocument();
  const card = screen.container.querySelector<HTMLElement>('[title="Bob"]')!;
  await expect
    .poll(() => card.querySelector('[data-testid="call-feed-local-mute-button"]'))
    .toBeNull();
  card.dispatchEvent(
    new PointerEvent('pointerdown', {
      bubbles: true,
      pointerType: 'touch',
      isPrimary: true,
      pointerId: 7,
      clientX: 20,
      clientY: 20
    })
  );
  await expect
    .poll(() =>
      document.querySelector(
        '[data-menu-presentation="sheet"] [data-testid="call-feed-local-mute-button"]'
      )
    )
    .not.toBeNull();
  card.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }));
  flushSync();
  expect(document.querySelectorAll('[data-testid="copy-user-id"]')).toHaveLength(1);
  expect(
    document
      .querySelector('[data-menu-presentation="sheet"] [data-testid="call-feed-local-mute-button"]')
      ?.getAttribute('aria-pressed')
  ).toBe('true');
  card.dispatchEvent(
    new PointerEvent('pointerup', { bubbles: true, pointerType: 'touch', pointerId: 7 })
  );
});

it('offers unpin first in a compact featured menu and restores focus after unpinning', async () => {
  const screen = renderCallPanelHarness({ layout: 'stage', scenario: 'screen' });
  await screen.getByRole('button', { name: 'Pin Bob to the stage' }).click();
  const featured = screen.container.querySelector<HTMLElement>(
    '[data-testid="call-featured-stage-card"]'
  )!;
  featured.style.width = '319px';
  await expect
    .poll(() => featured.querySelector('[data-testid="call-stage-unpin-button"]'))
    .toBeNull();
  featured
    .querySelector<HTMLButtonElement>('[data-testid="call-participant-menu-button"]')!
    .click();
  await expect
    .poll(() => document.querySelector('[popover] [data-testid="call-stage-unpin-button"]'))
    .not.toBeNull();
  const unpin = document.querySelector<HTMLButtonElement>(
    '[popover] [data-testid="call-stage-unpin-button"]'
  )!;
  const actions = Array.from(unpin.parentElement!.querySelectorAll('[data-testid]')).map(
    (element) => element.getAttribute('data-testid')
  );
  expect(actions[0]).toBe('call-stage-unpin-button');
  unpin.click();
  await expect.poll(() => document.querySelector('[data-testid="copy-user-id"]')).toBeNull();
  await expect
    .poll(() => document.activeElement?.getAttribute('data-testid'))
    .toBe('call-stage-pin-button');
});

it('updates network warnings independently of microphone activity and clears them on recovery', async () => {
  const screen = renderCallPanelHarness({ layout: 'sidebar', scenario: 'voice' });
  const call = serverUi(serverRegistry.getStore(serverRegistry.originServer!.id)).voiceCall;
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
  const screen = renderCallPanelHarness({ layout: 'sidebar', scenario: 'voice' });
  await screen.getByTestId('call-device-menu-button').click();
  expect(goto).toHaveBeenCalledWith(
    `/chat/${serverIdToSegment(serverRegistry.originServer!.id)}/settings/voice`
  );
});

it('removes voice activity and participant controls when a call becomes observed', async () => {
  const screen = renderCallPanelHarness({ layout: 'sidebar', scenario: 'voice' });
  await expect.element(screen.getByTestId('call-participant-panel')).toBeInTheDocument();
  const store = serverRegistry.getStore(serverRegistry.originServer!.id);
  vi.spyOn(serverUi(store).voiceCall, 'getAudioLevel').mockReturnValue({
    isSpeaking: true,
    audioLevel: 0.5
  });
  vi.spyOn(serverUi(store).activeCallRooms, 'has').mockReturnValue(true);
  vi.spyOn(serverUi(store).activeCallRooms, 'getParticipants').mockReturnValue([
    { userId: 'bob', login: 'bob', displayName: 'Bob', avatarUrl: null, isBot: false }
  ]);
  const bob = screen.container.querySelector<HTMLElement>('[title="Bob"]')!;
  await expect
    .poll(() => bob.querySelector('[data-testid="voice-activity"]')?.getAttribute('data-active'))
    .toBe('true');

  flushSync(() => {
    serverUi(store).voiceCall.connected = false;
    serverUi(store).voiceCall.roomId = null;
  });

  await expect.element(screen.getByTestId('call-observer-panel')).toBeInTheDocument();
  await expect.element(screen.getByTestId('call-join-button')).toBeInTheDocument();
  expect(screen.container.querySelector('[title="Bob"]')).toBe(bob);
  expect(bob.querySelector('[data-testid="voice-activity"]')).toBeNull();
  expect(screen.container.querySelector('[data-testid="call-feed-local-mute-button"]')).toBeNull();
});

it('settles voice activity when the participant mutes their microphone', async () => {
  const screen = renderCallPanelHarness({ layout: 'sidebar', scenario: 'voice' });
  await expect.element(screen.getByTestId('call-participant-panel')).toBeInTheDocument();
  const call = serverUi(serverRegistry.getStore(serverRegistry.originServer!.id)).voiceCall;
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
  const screen = renderCallPanelHarness({ layout: 'sidebar', scenario: 'voice' });
  await expect.element(screen.getByTestId('call-participant-panel')).toBeInTheDocument();
  const store = serverRegistry.getStore(serverRegistry.originServer!.id);
  const roomId = serverUi(store).voiceCall.roomId!;
  const room = store.projection.rooms.get(roomId)!;
  flushSync(() => {
    serverUi(store).voiceCall.isMuted = true;
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
  screen.container.style.width = '296px';
  await expect
    .poll(() => screen.container.querySelector('[data-testid="call-feed-local-mute-button"]'))
    .toBeNull();
  selfCard
    .querySelector<HTMLButtonElement>('[data-testid="call-participant-menu-button"]')!
    .click();
  await expect.poll(() => document.querySelector('[data-testid="copy-user-id"]')).not.toBeNull();
  expect(
    document.querySelector('[popover] [data-testid="call-feed-local-mute-button"]')
  ).toBeNull();
  flushSync(() => {
    serverUi(store).voiceCall.connected = false;
  });
  vi.spyOn(serverUi(store).activeCallRooms, 'has').mockReturnValue(false);
  await expect.element(screen.getByTestId('call-join-button')).toBeDisabled();
});

it('includes volume controls in the remote user context menu', async () => {
  const screen = renderCallPanelHarness({ layout: 'sidebar', scenario: 'voice' });
  await expect.element(screen.getByTestId('call-participant-panel')).toBeInTheDocument();
  const store = serverRegistry.getStore(serverRegistry.originServer!.id);
  const change = vi.spyOn(serverUi(store).voiceCall, 'setParticipantVolume');
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
    serverUi(store).voiceCall.connected = false;
  });
  expect(document.querySelector('input[type="range"]')).toBeNull();
});

it('opens the user menu and volume controls by right-clicking a media card', async () => {
  const screen = renderCallPanelHarness({ layout: 'stage', scenario: 'screen' });
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
  const call = serverUi(serverRegistry.getStore(serverRegistry.originServer!.id)).voiceCall;
  const change = vi.spyOn(call, 'setParticipantVolume');
  const input = document.querySelector<HTMLInputElement>('input[type="range"]')!;
  input.value = '60';
  input.dispatchEvent(new Event('input', { bubbles: true }));
  expect(change).toHaveBeenCalledWith('dana', 'streamVolume', 60);
});

it.each(['sidebar', 'stage'] as const)(
  'uses screen audio independently in the %s screen tile',
  async (layout) => {
    const screen = renderCallPanelHarness({ layout, scenario: 'screen' });
    await expect.element(screen.getByTestId('call-participant-panel')).toBeInTheDocument();
    const call = serverUi(serverRegistry.getStore(serverRegistry.originServer!.id)).voiceCall;
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
  const screen = renderCallPanelHarness({ layout: 'sidebar', scenario: 'voice' });
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
  const screen = renderCallPanelHarness({ layout: 'stage', scenario: 'screen' });
  await expect.element(screen.getByTestId('call-featured-stage-card')).toBeInTheDocument();
  const card = screen.container.querySelector('[data-testid="call-featured-stage-card"]')!;
  expect(card.querySelector('[data-testid="call-participant-menu-button"]')).not.toBeNull();
  expect(card.querySelector('input[type="range"]')).toBeNull();
});

it('keeps voice cards equal in height with compact direct mute controls', async () => {
  const screen = renderCallPanelHarness({ layout: 'sidebar', scenario: 'voice' });
  await expect.element(screen.getByTestId('call-participant-panel')).toBeInTheDocument();
  const store = serverRegistry.getStore(serverRegistry.originServer!.id);
  const muteRemote = vi.spyOn(serverUi(store).voiceCall, 'toggleParticipantLocalMute');
  const muteSelf = vi.spyOn(serverUi(store).voiceCall, 'toggleMute').mockResolvedValue();
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
  expect(selfMuteButton.getAttribute('aria-pressed')).toBe('false');
  expect(
    selfMuteButton.querySelector('.iconify')?.classList.contains('icon-[uil--microphone]')
  ).toBe(true);
  selfMuteButton.click();
  expect(muteSelf).toHaveBeenCalledOnce();
  flushSync(() => {
    serverUi(store).voiceCall.isMuted = true;
    serverUi(store).voiceCall.participants = serverUi(store).voiceCall.participants.map(
      (participant) => ({
        ...participant,
        isMuted: true
      })
    );
  });
  // The name stays stable; aria-pressed announces the muted state.
  expect(selfMuteButton.getAttribute('aria-label')).toBe('Mute');
  expect(selfMuteButton.getAttribute('aria-pressed')).toBe('true');
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
  const screen = renderCallPanelHarness({ layout: 'sidebar', scenario: 'screen-voice' });
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

it('pins a filmstrip tile to the stage until the viewer unpins it or its source ends', async () => {
  const screen = renderCallPanelHarness({ layout: 'stage', scenario: 'screen' });
  const featured = screen.getByTestId('call-featured-stage-card');
  await expect.element(featured).toHaveTextContent("Dana's screen");
  const strip = screen.getByTestId('call-secondary-stage-list');
  await expect.element(strip.getByTestId('call-stage-tile')).toHaveLength(4);

  await strip.getByRole('button', { name: 'Pin Bob to the stage' }).click();
  await expect.element(featured).toHaveTextContent('Bob');
  await expect.element(strip).toHaveTextContent("Dana's screen");
  await expect.element(strip).not.toHaveTextContent('Bob');

  await featured.getByTestId('call-stage-unpin-button').click();
  await expect.element(featured).toHaveTextContent("Dana's screen");

  await strip.getByRole('button', { name: 'Pin Chloe to the stage' }).click();
  await expect.element(featured).toHaveTextContent('Chloe');
  const call = serverUi(serverRegistry.getStore(serverRegistry.originServer!.id)).voiceCall;
  const chloe = call.participants.find((p) => p.identity === 'chloe')!;
  flushSync(() => {
    call.participants = call.participants.filter((p) => p.identity !== 'chloe');
  });
  await expect.element(featured).toHaveTextContent("Dana's screen");

  // The ended source's pin is cleared, so a returning participant stays in the strip.
  flushSync(() => {
    call.participants = [...call.participants, chloe];
  });
  await expect.element(strip).toHaveTextContent('Chloe');
  await expect.element(featured).toHaveTextContent("Dana's screen");
});

it.each(['Bob', 'Chloe', "Dana's screen"])(
  'unpins %s when the featured tile is clicked again',
  async (name) => {
    const screen = renderCallPanelHarness({ layout: 'stage', scenario: 'screen' });
    const featured = screen.getByTestId('call-featured-stage-card');
    const strip = screen.getByTestId('call-secondary-stage-list');
    if (name === "Dana's screen") {
      await strip.getByRole('button', { name: 'Pin Bob to the stage' }).click();
    }
    await strip.getByRole('button', { name: `Pin ${name} to the stage` }).click();
    await expect.element(featured).toHaveTextContent(name);

    // The media button is separate from the title bar's unpin action.
    const media = featured.element().querySelector<HTMLButtonElement>(':scope > button')!;
    expect(media.getAttribute('aria-label')).toBe(`Pin ${name} to the stage`);
    expect(media.getAttribute('aria-pressed')).toBe('true');
    media.click();

    await expect.element(featured).toHaveTextContent("Dana's screen");
    await expect.element(featured.getByTestId('call-stage-unpin-button')).not.toBeInTheDocument();
    expect(document.querySelector('[data-testid="copy-user-id"]')).toBeNull();
    await expect
      .poll(() => document.activeElement?.getAttribute('data-testid'))
      .toBe('call-stage-pin-button');
  }
);

it('toggles the automatic featured tile with left-click and opens its menu with right-click', async () => {
  const screen = renderCallPanelHarness({ layout: 'stage', scenario: 'screen' });
  const featured = screen.getByTestId('call-featured-stage-card');
  const media = featured.getByRole('button', { name: "Pin Dana's screen to the stage" });
  await expect.element(media).toHaveAttribute('aria-pressed', 'false');
  await media.click();
  await expect.element(media).toHaveAttribute('aria-pressed', 'true');
  expect(document.querySelector('[data-testid="copy-user-id"]')).toBeNull();
  await media.click();
  await expect.element(media).toHaveAttribute('aria-pressed', 'false');
  expect(document.querySelector('[data-testid="copy-user-id"]')).toBeNull();

  media.element().dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }));
  await expect.poll(() => document.querySelector('[data-testid="copy-user-id"]')).not.toBeNull();
  await expect.element(media).toHaveAttribute('aria-pressed', 'false');
});

it('keeps keyboard focus on the reversing control when pinning and unpinning', async () => {
  const screen = renderCallPanelHarness({ layout: 'stage', scenario: 'screen' });
  const strip = screen.getByTestId('call-secondary-stage-list');
  await strip.getByRole('button', { name: 'Pin Bob to the stage' }).click();
  await expect
    .poll(() => document.activeElement?.getAttribute('data-testid'))
    .toBe('call-stage-unpin-button');

  (document.activeElement as HTMLElement).click();
  await expect
    .poll(() => document.activeElement?.getAttribute('aria-label'))
    .toBe('Pin Bob to the stage');
});

it('features a remote screen share before the viewer screen share', async () => {
  const screen = renderCallPanelHarness({ layout: 'stage', scenario: 'screen' });
  await expect.element(screen.getByTestId('call-featured-stage-card')).toBeInTheDocument();
  const call = serverUi(serverRegistry.getStore(serverRegistry.originServer!.id)).voiceCall;
  const dana = call.participants.find((p) => p.identity === 'dana')!;
  flushSync(() => {
    call.participants = [
      ...call.participants
        .filter((p) => p.identity !== 'dana')
        .map((p) =>
          p.isLocal
            ? { ...p, isScreenShareEnabled: true, screenShareTrack: dana.screenShareTrack }
            : p
        ),
      dana
    ];
  });
  await expect
    .element(screen.getByTestId('call-featured-stage-card'))
    .toHaveTextContent("Dana's screen");
});

it('features remote camera feeds before the viewer camera', async () => {
  const screen = renderCallPanelHarness({ layout: 'stage', scenario: 'screen' });
  await expect.element(screen.getByTestId('call-featured-stage-card')).toBeInTheDocument();
  const call = serverUi(serverRegistry.getStore(serverRegistry.originServer!.id)).voiceCall;
  flushSync(() => {
    call.participants = call.participants.filter((p) => p.identity !== 'dana');
  });
  await expect.element(screen.getByTestId('call-featured-stage-card')).toHaveTextContent('Bob');
});

it.each([
  { width: 1600, height: 560, placement: 'side' },
  { width: 900, height: 800, placement: 'bottom' }
])(
  'places the other stage tiles where the featured card gets more room: $placement',
  async ({ width, height, placement }) => {
    const screen = renderCallPanelHarness({ layout: 'stage', scenario: 'screen' });
    Object.assign(screen.container.style, {
      display: 'flex',
      width: `${width}px`,
      height: `${height}px`
    });
    await expect
      .element(screen.getByTestId('call-stage-layout'))
      .toHaveAttribute('data-filmstrip-placement', placement);
    const featured = screen.getByTestId('call-featured-stage-card').element();
    const tile = screen.getByTestId('call-stage-tile').first().element();
    const featuredBox = featured.getBoundingClientRect();
    const tileBox = tile.getBoundingClientRect();
    if (placement === 'side') {
      expect(tileBox.left).toBeGreaterThanOrEqual(featuredBox.right);
      expect(tileBox.top).toBe(featuredBox.top);
    } else {
      expect(tileBox.top).toBeGreaterThanOrEqual(featuredBox.bottom);
    }
  }
);

it('keeps focus on the stage when an unpinned source stays featured', async () => {
  const screen = renderCallPanelHarness({ layout: 'stage', scenario: 'screen' });
  const featured = screen.getByTestId('call-featured-stage-card');
  await screen
    .getByTestId('call-secondary-stage-list')
    .getByRole('button', { name: 'Pin Bob to the stage' })
    .click();
  await expect.element(featured).toHaveTextContent('Bob');
  const call = serverUi(serverRegistry.getStore(serverRegistry.originServer!.id)).voiceCall;
  flushSync(() => {
    call.participants = call.participants.filter((p) => p.identity !== 'dana');
  });

  await featured.getByTestId('call-stage-unpin-button').click();
  await expect.element(featured).toHaveTextContent('Bob');
  await expect
    .poll(() => document.activeElement?.getAttribute('data-testid'))
    .toBe('call-stage-pin-button');
  expect(featured.element().contains(document.activeElement)).toBe(true);
});

it('features a remote active speaker without a camera over the viewer camera', async () => {
  const screen = renderCallPanelHarness({ layout: 'stage', scenario: 'camera' });
  const featured = screen.getByTestId('call-featured-stage-card');
  await expect.element(featured).toHaveTextContent('Alice');
  const call = serverUi(serverRegistry.getStore(serverRegistry.originServer!.id)).voiceCall;

  flushSync(() => {
    call.activeSpeakerIdentity = 'chloe';
  });
  await expect.element(featured).toHaveTextContent('Chloe');

  // A pin still wins over the active speaker.
  await screen
    .getByTestId('call-secondary-stage-list')
    .getByRole('button', { name: 'Pin Bob to the stage' })
    .click();
  await expect.element(featured).toHaveTextContent('Bob');
});

it('keeps screen shares ahead of the active speaker', async () => {
  const screen = renderCallPanelHarness({
    layout: 'stage',
    scenario: 'screen',
    activeSpeaker: 'chloe'
  });
  await expect
    .element(screen.getByTestId('call-featured-stage-card'))
    .toHaveTextContent("Dana's screen");
});

it('keeps the voice-only grid when someone speaks', async () => {
  const screen = renderCallPanelHarness({
    layout: 'stage',
    scenario: 'voice',
    activeSpeaker: 'bob'
  });
  await expect.element(screen.getByTestId('call-stage-grid')).toBeInTheDocument();
  expect(screen.container.querySelector('[data-testid="call-featured-stage-card"]')).toBeNull();
});

it('features the remote active speaker over another remote camera', async () => {
  const screen = renderCallPanelHarness({ layout: 'stage', scenario: 'screen' });
  const featured = screen.getByTestId('call-featured-stage-card');
  const call = serverUi(serverRegistry.getStore(serverRegistry.originServer!.id)).voiceCall;
  flushSync(() => {
    call.participants = call.participants.filter((p) => p.identity !== 'dana');
  });
  await expect.element(featured).toHaveTextContent('Bob');

  flushSync(() => {
    call.activeSpeakerIdentity = 'chloe';
  });
  await expect.element(featured).toHaveTextContent('Chloe');

  // A speaker who is no longer in the call does not block the fallback.
  flushSync(() => {
    call.participants = call.participants.filter((p) => p.identity !== 'chloe');
  });
  await expect.element(featured).toHaveTextContent('Bob');
});
