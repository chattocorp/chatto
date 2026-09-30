import '../../../app.css';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import { toast } from '$lib/ui/toast';
import VoiceCallPanelStoryHarness from './VoiceCallPanelStoryHarness.svelte';

let currentVideo: Element | null;

function enterPictureInPicture(element: HTMLVideoElement) {
  const previous = currentVideo;
  currentVideo = element;
  previous?.dispatchEvent(new Event('leavepictureinpicture'));
  element.dispatchEvent(new Event('enterpictureinpicture'));
}

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
  vi.spyOn(HTMLVideoElement.prototype, 'requestPictureInPicture').mockImplementation(
    async function (this: HTMLVideoElement) {
      enterPictureInPicture(this);
      return {} as PictureInPictureWindow;
    }
  );
  vi.spyOn(document, 'exitPictureInPicture').mockImplementation(async () => {
    const previous = currentVideo;
    currentVideo = null;
    previous?.dispatchEvent(new Event('leavepictureinpicture'));
  });
});

afterEach(() => {
  vi.restoreAllMocks();
  Object.defineProperty(HTMLVideoElement.prototype, 'requestPictureInPicture', requestDescriptor);
});

function mediaCards(container: HTMLElement) {
  return Array.from(container.querySelectorAll<HTMLElement>('[data-call-media-card]'));
}

function pipButton(card: HTMLElement) {
  return card.querySelector<HTMLButtonElement>('[data-testid="call-feed-pip-button"]')!;
}

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

it('only closes the PiP window owned by the removed tile', async () => {
  const screen = render(VoiceCallPanelStoryHarness, { props: { scenario: 'screen' } });
  await expect
    .poll(() => screen.container.querySelector('[data-testid="call-feed-pip-button"]'))
    .not.toBeNull();
  pipButton(mediaCards(screen.container)[0]).click();
  await expect.poll(() => currentVideo).not.toBeNull();
  await screen.unmount();
  expect(document.exitPictureInPicture).toHaveBeenCalledTimes(1);
  expect(currentVideo).toBeNull();
});
