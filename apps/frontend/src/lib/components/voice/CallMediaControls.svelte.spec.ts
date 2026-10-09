import '../../../app.css';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import { LocalVideoTrack } from 'livekit-client';
import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { endCallVideo, registerCallVideo, releaseCallVideo } from '$lib/state/callPictureInPicture';
import { toast } from '$lib/ui/toast';
import { serverRegistry } from '$lib/client';
import { serverUi } from '$lib/state/server/serverUi';
import { renderCallPanelHarness } from './renderCallPanelHarness';
import VideoThumbnail from './VideoThumbnail.svelte';

let currentVideo: Element | null;

class TestPictureInPictureWindow extends EventTarget {
  width = 480;
  height = 270;
  onresize: PictureInPictureWindow['onresize'] = null;
}

function enterPictureInPicture(element: HTMLVideoElement) {
  const previous = currentVideo;
  currentVideo = element;
  previous?.dispatchEvent(new Event('leavepictureinpicture'));
  const pipWindow = new TestPictureInPictureWindow();
  element.dispatchEvent(
    Object.assign(new Event('enterpictureinpicture'), { pictureInPictureWindow: pipWindow })
  );
  return pipWindow;
}

const moveDescriptor = Object.getOwnPropertyDescriptor(Element.prototype, 'moveBefore')!;
const requestDescriptor = Object.getOwnPropertyDescriptor(
  HTMLVideoElement.prototype,
  'requestPictureInPicture'
)!;

beforeEach(() => {
  currentVideo = null;
  vi.spyOn(document, 'pictureInPictureEnabled', 'get').mockReturnValue(true);
  vi.spyOn(document, 'pictureInPictureElement', 'get').mockImplementation(() => currentVideo);
  vi.spyOn(HTMLMediaElement.prototype, 'readyState', 'get').mockReturnValue(1);
  vi.spyOn(HTMLVideoElement.prototype, 'videoWidth', 'get').mockReturnValue(640);
  vi.spyOn(HTMLMediaElement.prototype, 'play').mockResolvedValue();
  vi.spyOn(HTMLMediaElement.prototype, 'paused', 'get').mockReturnValue(false);
  vi.spyOn(HTMLVideoElement.prototype, 'requestPictureInPicture').mockImplementation(
    async function (this: HTMLVideoElement) {
      return enterPictureInPicture(this);
    }
  );
  vi.spyOn(document, 'exitPictureInPicture').mockImplementation(async () => {
    const previous = currentVideo;
    currentVideo = null;
    previous?.dispatchEvent(new Event('leavepictureinpicture'));
  });
});

afterEach(() => {
  const previous = currentVideo;
  currentVideo = null;
  previous?.dispatchEvent(new Event('leavepictureinpicture'));
  vi.restoreAllMocks();
  Object.defineProperty(HTMLVideoElement.prototype, 'requestPictureInPicture', requestDescriptor);
  Object.defineProperty(Element.prototype, 'moveBefore', moveDescriptor);
});

it('keeps PiP through panel remounts and closes it when the call ends', async () => {
  const screen = renderCallPanelHarness({ scenario: 'screen', playableMedia: true });
  await expect.poll(() => mediaCards(screen.container).length).toBeGreaterThan(0);
  const video = mediaCards(screen.container)[0].querySelector('video')!;
  pipButton(mediaCards(screen.container)[0]).click();
  await expect.poll(() => currentVideo).toBe(video);
  await screen.getByRole('button', { name: 'Hide call panel' }).click();
  await expect.element(screen.getByRole('button', { name: 'Show call panel' })).toBeInTheDocument();
  await expect.poll(() => video.closest('[data-call-pip-host]')).not.toBeNull();
  expect(video.isConnected).toBe(true);
  expect(video.closest('[data-call-pip-host]')).not.toBeNull();
  expect(video.play).toHaveBeenCalled();
  await screen.getByRole('button', { name: 'Show call panel' }).click();
  const button = pipButton(mediaCards(screen.container)[0]);
  await expect.poll(() => button.getAttribute('aria-pressed')).toBe('true');
  button.click();
  await expect.poll(() => button.getAttribute('aria-pressed')).toBe('false');
  expect(video.isConnected).toBe(false);
  button.click();
  await expect.poll(() => currentVideo).not.toBeNull();
  await screen.getByRole('button', { name: 'Hide call panel' }).click();
  await screen.getByRole('button', { name: 'End call' }).click();
  expect(currentVideo).toBeNull();
  expect(document.querySelector('[data-call-pip-host]')).toBeNull();
});

it('moves the active video without restarting playback when moveBefore is available', async () => {
  const move = vi.spyOn(Element.prototype, 'moveBefore');
  const canvas = document.createElement('canvas');
  const track = new LocalVideoTrack(canvas.captureStream().getVideoTracks()[0]);
  const video = document.createElement('video');
  document.body.append(video);
  track.attach(video);
  registerCallVideo(track, video);
  vi.mocked(video.play).mockClear();
  try {
    enterPictureInPicture(video);
    releaseCallVideo(track, video);
    expect(move).toHaveBeenCalledWith(video, null);
    expect(video.isConnected).toBe(true);
    expect(video.play).not.toHaveBeenCalled();
  } finally {
    await document.exitPictureInPicture();
    track.stop();
    video.remove();
  }
});

it.each([false, true])('falls back to insertion while preserving paused=%s', async (paused) => {
  Object.defineProperty(Element.prototype, 'moveBefore', { configurable: true, value: undefined });
  vi.spyOn(HTMLMediaElement.prototype, 'paused', 'get').mockReturnValue(paused);
  const screen = renderCallPanelHarness({ scenario: 'camera' });
  await expect.poll(() => mediaCards(screen.container).length).toBeGreaterThan(0);
  const video = mediaCards(screen.container)[0].querySelector('video')!;
  enterPictureInPicture(video);
  await screen.unmount();
  expect(video.isConnected).toBe(true);
  expect(video.play).toHaveBeenCalledTimes(paused ? 0 : 1);
});

it.each(['browser', 'call'] as const)(
  'sizes retained video to native PiP and removes resize listeners on %s closure',
  async (closure) => {
    const screen = renderCallPanelHarness({ scenario: 'screen', playableMedia: true });
    await expect.poll(() => mediaCards(screen.container).length).toBeGreaterThan(0);
    const video = mediaCards(screen.container)[0].querySelector('video')!;
    const pipWindow = enterPictureInPicture(video);
    const removeListener = vi.spyOn(pipWindow, 'removeEventListener');
    pipWindow.width = 800;
    pipWindow.height = 450;
    await screen.getByRole('button', { name: 'Hide call panel' }).click();
    await expect
      .element(screen.getByRole('button', { name: 'Show call panel' }))
      .toBeInTheDocument();
    await expect.poll(() => video.closest('[data-call-pip-host]')).not.toBeNull();
    const host = video.closest<HTMLElement>('[data-call-pip-host]')!;
    expect(host.style.width).toBe('800px');
    expect(host.style.height).toBe('450px');
    expect(video.clientWidth).toBe(800);
    expect(video.clientHeight).toBe(450);
    pipWindow.width = 960;
    pipWindow.height = 540;
    pipWindow.dispatchEvent(new Event('resize'));
    expect(video.clientWidth).toBe(960);
    expect(video.clientHeight).toBe(540);
    if (closure === 'browser') await document.exitPictureInPicture();
    else await screen.getByRole('button', { name: 'End call' }).click();
    expect(removeListener).toHaveBeenCalledWith('resize', expect.any(Function));
    expect(host.isConnected).toBe(false);
    pipWindow.width = 1280;
    pipWindow.dispatchEvent(new Event('resize'));
    expect(host.style.width).toBe('960px');
  }
);

function mediaCards(container: HTMLElement) {
  return Array.from(container.querySelectorAll<HTMLElement>('[data-call-media-card]'));
}

function pipButton(card: HTMLElement) {
  return card.querySelector<HTMLButtonElement>('[data-testid="call-feed-pip-button"]')!;
}

/** Open through the actual video event path, including secondary stage tiles. */
async function openVideoMenu(video: HTMLVideoElement): Promise<HTMLButtonElement> {
  video.dispatchEvent(
    new MouseEvent('contextmenu', {
      bubbles: true,
      cancelable: true,
      button: 2,
      clientX: 120,
      clientY: 80
    })
  );
  flushSync();
  await expect
    .poll(() => document.querySelector('[data-testid="call-menu-pip-button"]'))
    .not.toBeNull();
  return document.querySelector<HTMLButtonElement>('[data-testid="call-menu-pip-button"]')!;
}

it.each([false, true])(
  'preserves PiP when responsive controls move from menu to header, pending=%s',
  async (pending) => {
    let finish!: () => void;
    if (pending) {
      vi.spyOn(HTMLVideoElement.prototype, 'requestPictureInPicture').mockImplementation(function (
        this: HTMLVideoElement
      ) {
        return new Promise((resolve) => {
          finish = () => resolve(enterPictureInPicture(this));
        });
      });
    }
    const screen = renderCallPanelHarness({
      layout: 'sidebar',
      scenario: 'screen',
      playableMedia: true
    });
    await expect.poll(() => mediaCards(screen.container).length).toBeGreaterThan(1);
    for (const card of mediaCards(screen.container)) {
      const video = card.querySelector('video')!;
      const stream = video.srcObject;
      card.style.width = '319px';
      await expect.poll(() => pipButton(card)).toBeNull();
      const menuButton = await openVideoMenu(video);
      expect(document.querySelector('[data-testid="call-feed-fullscreen-button"]')).toBeNull();
      menuButton.click();
      if (pending) {
        await expect.poll(() => menuButton.disabled).toBe(true);
      } else {
        await expect.poll(() => currentVideo).toBe(video);
        expect((await openVideoMenu(video)).getAttribute('aria-pressed')).toBe('true');
      }
      card.style.width = '320px';
      await expect.poll(() => document.querySelector('[data-testid="copy-user-id"]')).toBeNull();
      await expect.poll(() => pipButton(card)).not.toBeNull();
      if (pending) {
        expect(pipButton(card).disabled).toBe(true);
        finish();
      }
      await expect.poll(() => pipButton(card).getAttribute('aria-pressed')).toBe('true');
      expect(currentVideo).toBe(video);
      expect(card.querySelector('video')).toBe(video);
      expect(video.srcObject).toBe(stream);
      expect(document.exitPictureInPicture).not.toHaveBeenCalled();
      pipButton(card).click();
      await expect.poll(() => currentVideo).toBeNull();
      vi.mocked(document.exitPictureInPicture).mockClear();
    }
  }
);

it.each(['sidebar', 'stage'] as const)(
  'toggles the selected video from its menu and synchronizes header state in %s',
  async (layout) => {
    const screen = renderCallPanelHarness({ scenario: 'screen', layout, playableMedia: true });
    const tileVideo = (index: number) =>
      screen.container.querySelectorAll<HTMLVideoElement>('video')[index];
    await expect.poll(() => screen.container.querySelector('video')).not.toBeNull();
    const firstVideo = tileVideo(0);
    const menuButton = await openVideoMenu(firstVideo);
    expect(menuButton.getAttribute('aria-pressed')).toBe('false');
    menuButton.click();
    await expect.poll(() => currentVideo).toBe(firstVideo);
    await expect
      .poll(() => document.querySelector('[data-testid="call-menu-pip-button"]'))
      .toBeNull();
    await expect
      .poll(() => pipButton(mediaCards(screen.container)[0]).getAttribute('aria-pressed'))
      .toBe('true');

    // A panel remount retains the original PiP video outside its replacement tile.
    await screen.getByRole('button', { name: 'Hide call panel' }).click();
    await screen.getByRole('button', { name: 'Show call panel' }).click();
    expect(firstVideo.closest('[data-call-pip-host]')).not.toBeNull();

    const activeMenu = await openVideoMenu(tileVideo(0));
    expect(activeMenu.getAttribute('aria-pressed')).toBe('true');
    await document.exitPictureInPicture();
    await expect.poll(() => activeMenu.getAttribute('aria-pressed')).toBe('false');
    const nextFirstVideo = tileVideo(0);
    activeMenu.click();
    await expect.poll(() => currentVideo).toBe(nextFirstVideo);

    const secondVideo = tileVideo(1);
    (await openVideoMenu(tileVideo(1))).click();
    await expect.poll(() => currentVideo).toBe(secondVideo);
    expect(pipButton(mediaCards(screen.container)[0]).getAttribute('aria-pressed')).toBe('false');
    (await openVideoMenu(tileVideo(1))).click();
    await expect.poll(() => currentVideo).toBeNull();
  }
);

it('shares readiness and pending state between the user menu and header', async () => {
  const readiness = vi.spyOn(HTMLMediaElement.prototype, 'readyState', 'get').mockReturnValue(0);
  let completeRequest!: () => void;
  const request = vi
    .spyOn(HTMLVideoElement.prototype, 'requestPictureInPicture')
    .mockImplementation(function (this: HTMLVideoElement) {
      return new Promise((resolve) => {
        completeRequest = () => resolve(enterPictureInPicture(this));
      });
    });
  const screen = renderCallPanelHarness({ scenario: 'camera', layout: 'sidebar' });
  await expect.poll(() => mediaCards(screen.container).length).toBeGreaterThan(0);
  const card = mediaCards(screen.container)[0];
  const video = card.querySelector('video')!;
  const menuButton = await openVideoMenu(video);
  expect(menuButton.disabled).toBe(true);
  readiness.mockReturnValue(1);
  video.dispatchEvent(new Event('loadedmetadata'));
  await expect.poll(() => menuButton.disabled).toBe(false);
  menuButton.click();
  pipButton(card).click();
  await expect.poll(() => pipButton(card).disabled).toBe(true);
  expect(menuButton.disabled).toBe(true);
  expect(request).toHaveBeenCalledTimes(1);
  // Dismissing this UI owner does not end the still-mounted video's request.
  window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
  await expect
    .poll(() => document.querySelector('[data-testid="call-menu-pip-button"]'))
    .toBeNull();
  completeRequest();
  await expect.poll(() => currentVideo).toBe(video);
  expect(document.exitPictureInPicture).not.toHaveBeenCalled();
  await expect.poll(() => pipButton(card).getAttribute('aria-pressed')).toBe('true');
});

it.each(['same', 'another'] as const)(
  'keeps a reopened menu for %s video open when an earlier request completes',
  async (source) => {
    let completeRequest!: () => void;
    vi.spyOn(HTMLVideoElement.prototype, 'requestPictureInPicture').mockImplementation(function (
      this: HTMLVideoElement
    ) {
      return new Promise((resolve) => {
        completeRequest = () => resolve(enterPictureInPicture(this));
      });
    });
    const screen = renderCallPanelHarness({ scenario: 'screen', layout: 'sidebar' });
    await expect.poll(() => mediaCards(screen.container).length).toBeGreaterThan(1);
    const [first, second] = screen.container.querySelectorAll('video');
    (await openVideoMenu(first)).click();
    const selectedMenu = await openVideoMenu(source === 'same' ? first : second);
    completeRequest();
    await expect.poll(() => currentVideo).toBe(first);
    expect(document.querySelector('[data-testid="call-menu-pip-button"]')).toBe(selectedMenu);
    expect(selectedMenu.getAttribute('aria-pressed')).toBe(source === 'same' ? 'true' : 'false');
  }
);

it('releases retained video state when the browser closes PiP after a panel remount', async () => {
  const screen = renderCallPanelHarness({ scenario: 'screen', playableMedia: true });
  await expect.poll(() => screen.container.querySelector('video')).not.toBeNull();
  const original = screen.container.querySelector('video')!;
  (await openVideoMenu(original)).click();
  await expect.poll(() => currentVideo).toBe(original);
  await screen.getByRole('button', { name: 'Hide call panel' }).click();
  await screen.getByRole('button', { name: 'Show call panel' }).click();
  await expect.poll(() => screen.container.querySelector('video')).not.toBeNull();
  const remounted = screen.container.querySelector('video')!;
  const button = await openVideoMenu(remounted);
  expect(button.getAttribute('aria-pressed')).toBe('true');
  await document.exitPictureInPicture();
  await expect.poll(() => button.getAttribute('aria-pressed')).toBe('false');
  expect(button.disabled).toBe(false);
  button.click();
  await expect.poll(() => currentVideo).toBe(remounted);
});

it.each(['enter', 'exit'] as const)(
  'keeps the menu usable after a rejected PiP %s request',
  async (operation) => {
    const error = vi.spyOn(toast, 'error').mockReturnValue('pip-error');
    const screen = renderCallPanelHarness({ scenario: 'camera' });
    await expect.poll(() => screen.container.querySelector('video')).not.toBeNull();
    const video = screen.container.querySelector('video')!;
    if (operation === 'exit') {
      enterPictureInPicture(video);
      vi.spyOn(document, 'exitPictureInPicture').mockRejectedValue(new DOMException('raw failure'));
    } else {
      vi.spyOn(HTMLVideoElement.prototype, 'requestPictureInPicture').mockRejectedValue(
        new DOMException('raw failure')
      );
    }
    const button = await openVideoMenu(video);
    button.click();
    await expect
      .poll(() => error.mock.calls)
      .toEqual([['Could not change picture-in-picture mode.']]);
    expect(document.querySelector('[data-testid="call-menu-pip-button"]')).toBe(button);
    expect(button.disabled).toBe(false);
  }
);

it('discards a late menu PiP request after its call stream ends', async () => {
  let completeRequest!: () => void;
  vi.spyOn(HTMLVideoElement.prototype, 'requestPictureInPicture').mockImplementation(function (
    this: HTMLVideoElement
  ) {
    return new Promise((resolve) => {
      completeRequest = () => resolve(enterPictureInPicture(this));
    });
  });
  const error = vi.spyOn(toast, 'error').mockReturnValue('pip-error');
  const screen = renderCallPanelHarness({ scenario: 'camera', playableMedia: true });
  await expect.poll(() => screen.container.querySelector('video')).not.toBeNull();
  const video = screen.container.querySelector('video')!;
  (await openVideoMenu(video)).click();
  await screen.getByRole('button', { name: 'End call' }).click();
  completeRequest();
  await expect.poll(() => document.exitPictureInPicture).toHaveBeenCalledTimes(1);
  expect(currentVideo).toBeNull();
  expect(error).not.toHaveBeenCalled();
});

it('disables a selected stream that ends while its menu is open', async () => {
  const screen = renderCallPanelHarness({ scenario: 'camera' });
  await expect.poll(() => screen.container.querySelector('video')).not.toBeNull();
  const video = screen.container.querySelector('video')!;
  const button = await openVideoMenu(video);
  const call = serverUi(serverRegistry.getStore(serverRegistry.originServer!.id)).voiceCall;
  endCallVideo(call.participants.find((participant) => participant.isLocal)!.videoTrack!);
  await expect.poll(() => button.disabled).toBe(true);
  button.click();
  expect(HTMLVideoElement.prototype.requestPictureInPicture).not.toHaveBeenCalled();
});

it('omits video actions from voice-only participant menus', async () => {
  const screen = renderCallPanelHarness({ scenario: 'voice' });
  await expect
    .poll(() => screen.container.querySelector('[data-testid="call-participant-menu-button"]'))
    .not.toBeNull();
  screen.container
    .querySelector<HTMLButtonElement>('[data-testid="call-participant-menu-button"]')!
    .click();
  await expect.poll(() => document.querySelector('[data-testid="copy-user-id"]')).not.toBeNull();
  expect(document.querySelector('[data-testid="call-menu-pip-button"]')).toBeNull();
});

it('retains the original PiP stream when a tile is reused for another track', async () => {
  const original = new LocalVideoTrack(
    document.createElement('canvas').captureStream().getVideoTracks()[0]
  );
  const replacement = new LocalVideoTrack(
    document.createElement('canvas').captureStream().getVideoTracks()[0]
  );
  const screen = render(VideoThumbnail, {
    props: {
      track: original,
      name: 'Participant',
      showIdentityOverlay: false,
      user: {
        id: 'participant',
        login: 'participant',
        displayName: 'Participant',
        isBot: false,
        deleted: false,
        avatarUrl: null,
        presenceStatus: PresenceStatus.OFFLINE
      }
    }
  });
  try {
    const video = screen.container.querySelector('video')!;
    enterPictureInPicture(video);
    await screen.rerender({ track: replacement });
    expect(currentVideo).toBe(video);
    expect(document.exitPictureInPicture).not.toHaveBeenCalled();
    expect(video.isConnected).toBe(true);
    expect(original.attachedElements).toContain(video);
    expect(screen.container.querySelector('video')).not.toBe(video);
    expect(replacement.attachedElements).toContain(screen.container.querySelector('video'));
  } finally {
    await document.exitPictureInPicture();
    await screen.unmount();
    original.stop();
    replacement.stop();
  }
});

it.each([
  ['sidebar', 'screen'],
  ['stage', 'screen'],
  ['sidebar', 'camera'],
  ['stage', 'camera']
] as const)('opens user menus from video right-clicks in %s %s tiles', async (layout, scenario) => {
  const screen = renderCallPanelHarness({ layout, scenario });
  await expect.poll(() => mediaCards(screen.container).length).toBeGreaterThan(0);
  const cards = Array.from(
    screen.container.querySelectorAll<HTMLElement>('[data-call-tile]')
  ).filter((card) => card.querySelector('video'));
  for (const card of cards) {
    expect(card.querySelector('[data-testid="call-feed-fullscreen-button"]')).toBeNull();
    const event = new MouseEvent('contextmenu', { bubbles: true, cancelable: true, button: 2 });
    card.querySelector('video')!.dispatchEvent(event);
    flushSync();
    expect(event.defaultPrevented).toBe(true);
    await expect.poll(() => document.querySelector('[data-testid="copy-user-id"]')).not.toBeNull();
    await expect
      .poll(() => document.querySelector('[data-testid="call-menu-pip-button"]'))
      .not.toBeNull();
    expect(document.querySelectorAll('[data-testid="copy-user-id"]')).toHaveLength(1);
    expect(document.querySelector('[role="dialog"]')?.textContent).toContain(
      card.querySelector('video')!.title.replace(/'s screen$/, '')
    );
    const screenShare =
      card.dataset.testid === 'call-screen-share-card' ||
      card.dataset.stageTileKind === 'screen' ||
      card.title.includes('screen');
    if (screenShare) {
      await expect
        .element(screen.getByRole('slider', { name: /Stream volume/ }))
        .toBeInTheDocument();
    } else if (card.querySelector('video')!.title === 'Bob') {
      await expect
        .element(screen.getByRole('slider', { name: /Voice volume/ }))
        .toBeInTheDocument();
    }
  }
  mediaCards(screen.container)[0].querySelector('video')!.click();
  await expect.poll(() => document.querySelector('[data-testid="copy-user-id"]')).not.toBeNull();
});

it('preserves touch long-press and suppresses its duplicate native menu', async () => {
  const screen = renderCallPanelHarness({ scenario: 'screen' });
  await expect.poll(() => screen.container.querySelector('video')).not.toBeNull();
  const video = screen.container.querySelector('video')!;
  video.dispatchEvent(
    new PointerEvent('pointerdown', {
      bubbles: true,
      pointerType: 'touch',
      pointerId: 7,
      isPrimary: true,
      clientX: 120,
      clientY: 80
    })
  );
  await expect.poll(() => document.querySelector('[data-testid="copy-user-id"]')).not.toBeNull();
  const event = new MouseEvent('contextmenu', { bubbles: true, cancelable: true });
  video.dispatchEvent(event);
  expect(event.defaultPrevented).toBe(true);
  expect(document.querySelectorAll('[data-testid="copy-user-id"]')).toHaveLength(1);
  video.dispatchEvent(
    new PointerEvent('pointerup', { bubbles: true, pointerType: 'touch', pointerId: 7 })
  );
});

it('toggles the selected video and follows browser closure in the sidebar', async () => {
  const screen = renderCallPanelHarness({ layout: 'sidebar', scenario: 'screen' });
  screen.container.style.width = '800px';
  await expect
    .poll(() => screen.container.querySelectorAll('[data-testid="call-feed-pip-button"]').length)
    .toBeGreaterThan(1);
  const [screenCard, cameraCard] = mediaCards(screen.container);
  pipButton(screenCard).click();
  await expect.poll(() => pipButton(screenCard).getAttribute('aria-pressed')).toBe('true');
  expect(currentVideo).toBe(screenCard.querySelector('video'));
  expect(document.querySelector('[data-testid="copy-user-id"]')).toBeNull();
  pipButton(cameraCard).click();
  await expect.poll(() => pipButton(cameraCard).getAttribute('aria-pressed')).toBe('true');
  expect(currentVideo).toBe(cameraCard.querySelector('video'));
  expect(pipButton(screenCard).getAttribute('aria-pressed')).toBe('false');
  pipButton(cameraCard).click();
  await expect.poll(() => pipButton(cameraCard).getAttribute('aria-pressed')).toBe('false');
  expect(currentVideo).toBeNull();
  pipButton(screenCard).click();
  await expect.poll(() => pipButton(screenCard).getAttribute('aria-pressed')).toBe('true');
  await document.exitPictureInPicture();
  await expect.poll(() => pipButton(screenCard).getAttribute('aria-pressed')).toBe('false');
});

it('keeps media controls on the featured stage source and off the filmstrip', async () => {
  const screen = renderCallPanelHarness({ layout: 'stage', scenario: 'screen' });
  await expect
    .poll(() => screen.container.querySelector('[data-testid="call-feed-pip-button"]'))
    .not.toBeNull();
  const [featured, ...others] = mediaCards(screen.container);
  expect(featured.dataset.testid).toBe('call-featured-stage-card');
  expect(others).toHaveLength(0);
  expect(
    screen.container.querySelector(
      '[data-testid="call-secondary-stage-list"] [data-testid="call-feed-pip-button"]'
    )
  ).toBeNull();
  pipButton(featured).click();
  await expect.poll(() => pipButton(featured).getAttribute('aria-pressed')).toBe('true');
  expect(currentVideo).toBe(featured.querySelector('video'));
  await document.exitPictureInPicture();
  await expect.poll(() => pipButton(featured).getAttribute('aria-pressed')).toBe('false');
});

it.each(['policy', 'method'] as const)(
  'hides PiP when unsupported by %s and still opens the user menu',
  async (reason) => {
    if (reason === 'policy') {
      vi.spyOn(document, 'pictureInPictureEnabled', 'get').mockReturnValue(false);
    } else {
      Object.defineProperty(HTMLVideoElement.prototype, 'requestPictureInPicture', {
        value: undefined,
        configurable: true
      });
    }
    const screen = renderCallPanelHarness({ scenario: 'screen' });
    await expect.poll(() => screen.container.querySelector('video')).not.toBeNull();
    flushSync();
    expect(screen.container.querySelector('[data-testid="call-feed-pip-button"]')).toBeNull();
    const event = new MouseEvent('contextmenu', { bubbles: true, cancelable: true });
    screen.container.querySelector('video')!.dispatchEvent(event);
    expect(event.defaultPrevented).toBe(true);
    await expect.poll(() => document.querySelector('[data-testid="copy-user-id"]')).not.toBeNull();
    expect(document.querySelector('[data-testid="call-menu-pip-button"]')).toBeNull();
  }
);

it('waits for media and prevents duplicate requests while a request is pending', async () => {
  const readiness = vi.spyOn(HTMLMediaElement.prototype, 'readyState', 'get').mockReturnValue(0);
  let resolveRequest!: (value: PictureInPictureWindow) => void;
  const request = vi
    .spyOn(HTMLVideoElement.prototype, 'requestPictureInPicture')
    .mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveRequest = resolve;
        })
    );
  const screen = renderCallPanelHarness({ scenario: 'camera' });
  await expect
    .poll(() => screen.container.querySelector('[data-testid="call-feed-pip-button"]'))
    .not.toBeNull();
  const card = mediaCards(screen.container)[0];
  const button = pipButton(card);
  expect(button.disabled).toBe(true);
  readiness.mockReturnValue(1);
  card.querySelector('video')!.dispatchEvent(new Event('loadedmetadata'));
  await expect.poll(() => button.disabled).toBe(false);
  button.click();
  button.click();
  await expect.poll(() => button.disabled).toBe(true);
  expect(request).toHaveBeenCalledTimes(1);
  resolveRequest({} as PictureInPictureWindow);
  await expect.poll(() => button.disabled).toBe(false);
  readiness.mockReturnValue(0);
  card.querySelector('video')!.dispatchEvent(new Event('emptied'));
  await expect.poll(() => button.disabled).toBe(true);
});

it.each(['enter', 'exit'] as const)(
  'reports rejected %s requests without raw browser errors',
  async (operation) => {
    const error = vi.spyOn(toast, 'error').mockReturnValue('pip-error');
    const screen = renderCallPanelHarness({ scenario: 'camera' });
    await expect
      .poll(() => screen.container.querySelector('[data-testid="call-feed-pip-button"]'))
      .not.toBeNull();
    const button = pipButton(mediaCards(screen.container)[0]);
    if (operation === 'exit') {
      button.click();
      await expect.poll(() => button.getAttribute('aria-pressed')).toBe('true');
      vi.spyOn(document, 'exitPictureInPicture').mockRejectedValue(new DOMException('raw failure'));
    } else {
      vi.spyOn(HTMLVideoElement.prototype, 'requestPictureInPicture').mockRejectedValue(
        new DOMException('raw failure')
      );
    }
    button.click();
    await expect
      .poll(() => error.mock.calls)
      .toEqual([['Could not change picture-in-picture mode.']]);
    expect(button.disabled).toBe(false);
  }
);

it('closes its PiP window when unmounted, including a late successful request', async () => {
  let completeRequest!: () => void;
  vi.spyOn(HTMLVideoElement.prototype, 'requestPictureInPicture').mockImplementation(function (
    this: HTMLVideoElement
  ) {
    return new Promise((resolve) => {
      completeRequest = () => {
        enterPictureInPicture(this);
        resolve({} as PictureInPictureWindow);
      };
    });
  });
  const screen = renderCallPanelHarness({ scenario: 'camera' });
  await expect
    .poll(() => screen.container.querySelector('[data-testid="call-feed-pip-button"]'))
    .not.toBeNull();
  pipButton(mediaCards(screen.container)[0]).click();
  await screen.unmount();
  completeRequest();
  await expect.poll(() => document.exitPictureInPicture).toHaveBeenCalledTimes(1);
  expect(currentVideo).toBeNull();
});

it.each(['sidebar', 'stage'] as const)(
  'retains native PiP after %s tiles unmount',
  async (layout) => {
    const screen = renderCallPanelHarness({ scenario: 'screen', layout });
    await expect
      .poll(() => screen.container.querySelector('[data-testid="call-feed-pip-button"]'))
      .not.toBeNull();
    const video = mediaCards(screen.container)[0].querySelector('video')!;
    enterPictureInPicture(video);
    await expect.poll(() => currentVideo).not.toBeNull();
    await screen.unmount();
    expect(document.exitPictureInPicture).not.toHaveBeenCalled();
    expect(currentVideo).toBe(video);
    expect(video.isConnected).toBe(true);
    expect(video.closest('[data-call-pip-host]')).not.toBeNull();
    await document.exitPictureInPicture();
    expect(currentVideo).toBeNull();
    expect(video.isConnected).toBe(false);
    expect(document.querySelector('[data-call-pip-host]')).toBeNull();
  }
);

/** Rebuild every participant around the same tracks, as each LiveKit mute event does. */
function rebuildParticipants() {
  const call = serverUi(serverRegistry.getStore(serverRegistry.originServer!.id)).voiceCall;
  call.participants = call.participants.map((participant) => ({
    ...participant,
    isMuted: !participant.isMuted
  }));
  flushSync();
}

it.each(['sidebar', 'stage'] as const)(
  'keeps %s videos attached when participant state changes without new tracks',
  async (layout) => {
    const screen = renderCallPanelHarness({ layout, scenario: 'camera' });
    await expect.poll(() => mediaCards(screen.container).length).toBeGreaterThan(0);
    const videos = Array.from(screen.container.querySelectorAll('video'));
    // Harness tracks remove the poster on detach and set it again on attach.
    const changes: MutationRecord[] = [];
    const observer = new MutationObserver((records) => changes.push(...records));
    observer.observe(screen.container, { subtree: true, attributeFilter: ['poster'] });
    try {
      rebuildParticipants();
      changes.push(...observer.takeRecords());
      expect(changes).toEqual([]);
      const current = Array.from(screen.container.querySelectorAll('video'));
      expect(current).toHaveLength(videos.length);
      current.forEach((video, index) => expect(video).toBe(videos[index]));
    } finally {
      observer.disconnect();
    }
  }
);

it('keeps a picture-in-picture video in its tile when participant state changes', async () => {
  const screen = renderCallPanelHarness({ scenario: 'camera' });
  await expect.poll(() => mediaCards(screen.container).length).toBeGreaterThan(0);
  const video = mediaCards(screen.container)[0].querySelector('video')!;
  enterPictureInPicture(video);
  rebuildParticipants();
  expect(video.closest('[data-call-pip-host]')).toBeNull();
  expect(mediaCards(screen.container)[0].querySelector('video')).toBe(video);
});
