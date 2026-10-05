import '../../../app.css';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import { LocalVideoTrack } from 'livekit-client';
import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { registerCallVideo, releaseCallVideo } from '$lib/state/callPictureInPicture';
import { toast } from '$lib/ui/toast';
import VoiceCallPanelStoryHarness from './VoiceCallPanelStoryHarness.svelte';
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
  const screen = render(VoiceCallPanelStoryHarness, {
    props: { scenario: 'screen', playableMedia: true }
  });
  await expect.poll(() => mediaCards(screen.container).length).toBeGreaterThan(0);
  const video = mediaCards(screen.container)[0].querySelector('video')!;
  pipButton(mediaCards(screen.container)[0]).click();
  await expect.poll(() => currentVideo).toBe(video);
  await screen.getByRole('button', { name: 'Hide call panel' }).click();
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
  const screen = render(VoiceCallPanelStoryHarness, { props: { scenario: 'camera' } });
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
    const screen = render(VoiceCallPanelStoryHarness, {
      props: { scenario: 'screen', playableMedia: true }
    });
    await expect.poll(() => mediaCards(screen.container).length).toBeGreaterThan(0);
    const video = mediaCards(screen.container)[0].querySelector('video')!;
    const pipWindow = enterPictureInPicture(video);
    const removeListener = vi.spyOn(pipWindow, 'removeEventListener');
    pipWindow.width = 800;
    pipWindow.height = 450;
    await screen.getByRole('button', { name: 'Hide call panel' }).click();
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
] as const)(
  'preserves native video menus and left-click menus in %s %s tiles',
  async (layout, scenario) => {
    const screen = render(VoiceCallPanelStoryHarness, { props: { layout, scenario } });
    await expect.poll(() => mediaCards(screen.container).length).toBeGreaterThan(0);
    for (const card of mediaCards(screen.container)) {
      const event = new MouseEvent('contextmenu', { bubbles: true, cancelable: true, button: 2 });
      card.querySelector('video')!.dispatchEvent(event);
      flushSync();
      expect(event.defaultPrevented).toBe(false);
      expect(document.querySelector('[data-testid="copy-user-id"]')).toBeNull();
    }
    mediaCards(screen.container)[0].querySelector('video')!.click();
    await expect.poll(() => document.querySelector('[data-testid="copy-user-id"]')).not.toBeNull();
  }
);

it('preserves touch long-press and suppresses its duplicate native menu', async () => {
  const screen = render(VoiceCallPanelStoryHarness, { props: { scenario: 'screen' } });
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

it.each(['sidebar', 'stage'] as const)(
  'toggles the selected video and follows browser closure in %s',
  async (layout) => {
    const screen = render(VoiceCallPanelStoryHarness, { props: { layout, scenario: 'screen' } });
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
  }
);

it.each(['policy', 'method'] as const)(
  'hides PiP when unsupported by %s without blocking native menus',
  async (reason) => {
    if (reason === 'policy') {
      vi.spyOn(document, 'pictureInPictureEnabled', 'get').mockReturnValue(false);
    } else {
      Object.defineProperty(HTMLVideoElement.prototype, 'requestPictureInPicture', {
        value: undefined,
        configurable: true
      });
    }
    const screen = render(VoiceCallPanelStoryHarness, { props: { scenario: 'screen' } });
    await expect.poll(() => screen.container.querySelector('video')).not.toBeNull();
    flushSync();
    expect(screen.container.querySelector('[data-testid="call-feed-pip-button"]')).toBeNull();
    const event = new MouseEvent('contextmenu', { bubbles: true, cancelable: true });
    screen.container.querySelector('video')!.dispatchEvent(event);
    expect(event.defaultPrevented).toBe(false);
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
  const screen = render(VoiceCallPanelStoryHarness, { props: { scenario: 'camera' } });
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
    const screen = render(VoiceCallPanelStoryHarness, { props: { scenario: 'camera' } });
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
  const screen = render(VoiceCallPanelStoryHarness, { props: { scenario: 'camera' } });
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
    const screen = render(VoiceCallPanelStoryHarness, { props: { scenario: 'screen', layout } });
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
