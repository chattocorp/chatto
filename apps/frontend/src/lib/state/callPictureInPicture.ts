import type { Track } from 'livekit-client';
import { m } from '$lib/i18n/messages';
import { toastError } from '$lib/utils/errorMessage';

/**
 * Retain the original PiP video outside route-owned tiles until its stream or PiP ends.
 * The retained video lives in a hidden host on `document.body`, outside the room and
 * server route trees. Track identity separates calls and servers. Nothing is persisted.
 */
const videoTracks = new WeakMap<HTMLVideoElement, Track>();
const pictureInPictureWindows = new WeakMap<HTMLVideoElement, PictureInPictureWindow>();
const endedTracks = new WeakSet<Track>();
const retained = new Map<Track, { video: HTMLVideoElement; dispose: () => void }>();
const pendingVideos = new WeakSet<HTMLVideoElement>();
const changes = new EventTarget();

/** Browser state shared by a call video's header button and user-menu entry. */
export type CallPictureInPictureStatus = {
  supported: boolean;
  ready: boolean;
  active: boolean;
  pending: boolean;
  /** False when the selected tile or its call-owned stream has ended. */
  available: boolean;
};

function notifyChange(): void {
  changes.dispatchEvent(new Event('change'));
}

/** Read the retained video, so controls follow PiP across tile remounts. */
function pictureInPictureStatus(video: HTMLVideoElement): CallPictureInPictureStatus {
  const element = pictureInPictureVideo(video);
  const track = videoTracks.get(element);
  return {
    supported:
      typeof element.requestPictureInPicture === 'function' &&
      document.pictureInPictureEnabled &&
      !element.disablePictureInPicture,
    ready: element.readyState >= HTMLMediaElement.HAVE_METADATA && element.videoWidth > 0,
    active: document.pictureInPictureElement === element,
    pending: pendingVideos.has(element),
    available: !!track && !endedTracks.has(track) && element.isConnected
  };
}

/** Observe media readiness, browser PiP changes, and shared requests until cleanup. */
export function observeCallPictureInPicture(
  video: HTMLVideoElement,
  onchange: (status: CallPictureInPictureStatus) => void
): () => void {
  const update = () => onchange(pictureInPictureStatus(video));
  const events = ['loadedmetadata', 'loadeddata', 'resize', 'emptied'];
  for (const event of events) video.addEventListener(event, update);
  document.addEventListener('enterpictureinpicture', update, true);
  document.addEventListener('leavepictureinpicture', update, true);
  changes.addEventListener('change', update);
  update();
  return () => {
    for (const event of events) video.removeEventListener(event, update);
    document.removeEventListener('enterpictureinpicture', update, true);
    document.removeEventListener('leavepictureinpicture', update, true);
    changes.removeEventListener('change', update);
  };
}

/**
 * Toggle from a user click, without an async step before the browser request.
 * Requests belong to the video, so closing a menu does not cancel them.
 * Return true after success. Rejections show the call's localized error;
 * removed tiles and ended streams discard late results without an error.
 */
export async function toggleCallPictureInPicture(video: HTMLVideoElement): Promise<boolean> {
  const element = pictureInPictureVideo(video);
  const status = pictureInPictureStatus(video);
  if (
    !status.available ||
    !status.supported ||
    status.pending ||
    (!status.ready && !status.active)
  ) {
    return false;
  }
  const track = videoTracks.get(element)!;
  const available = () =>
    videoTracks.get(element) === track && !endedTracks.has(track) && element.isConnected;
  pendingVideos.add(element);
  notifyChange();
  try {
    if (document.pictureInPictureElement === element) {
      await document.exitPictureInPicture();
    } else {
      await element.requestPictureInPicture();
      if (!available()) {
        if (document.pictureInPictureElement === element) await document.exitPictureInPicture();
        return false;
      }
    }
    return true;
  } catch {
    if (available()) toastError(null, m('voice.picture_in_picture_failed'));
    return false;
  } finally {
    pendingVideos.delete(element);
    notifyChange();
  }
}

/** Observe browser-menu entry as well as requests made by the tile button. */
function rememberPictureInPictureWindow(event: PictureInPictureEvent): void {
  if (event.currentTarget instanceof HTMLVideoElement) {
    pictureInPictureWindows.set(event.currentTarget, event.pictureInPictureWindow);
  }
}

function forgetPictureInPictureWindow(event: Event): void {
  if (event.currentTarget instanceof HTMLVideoElement) {
    pictureInPictureWindows.delete(event.currentTarget);
  }
}

function unregisterCallVideo(video: HTMLVideoElement): void {
  video.removeEventListener('enterpictureinpicture', rememberPictureInPictureWindow);
  video.removeEventListener('leavepictureinpicture', forgetPictureInPictureWindow);
  pictureInPictureWindows.delete(video);
  videoTracks.delete(video);
  notifyChange();
}

/** Associate a tile video with its call-owned track without keeping either alive. */
export function registerCallVideo(track: Track, video: HTMLVideoElement): void {
  endedTracks.delete(track);
  videoTracks.set(video, track);
  video.addEventListener('enterpictureinpicture', rememberPictureInPictureWindow);
  video.addEventListener('leavepictureinpicture', forgetPictureInPictureWindow);
  notifyChange();
}

/** Resolve a remounted tile to its original video while that video is in PiP. */
export function pictureInPictureVideo(video: HTMLVideoElement): HTMLVideoElement {
  const track = videoTracks.get(video);
  return (track && retained.get(track)?.video) || video;
}

/** Release a tile's attachment, or move its active PiP video to a hidden DOM owner. */
export function releaseCallVideo(track: Track, video: HTMLVideoElement): void {
  if (document.pictureInPictureElement !== video || endedTracks.has(track)) {
    unregisterCallVideo(video);
    track.detach(video);
    return;
  }

  const host = document.createElement('div');
  host.setAttribute('aria-hidden', 'true');
  host.dataset.callPipHost = '';
  host.style.cssText = 'position:fixed;left:-10000px;top:0;visibility:hidden;pointer-events:none';
  const pipWindow = pictureInPictureWindows.get(video);
  // LiveKit's adaptive stream selects a video layer from the element size, so
  // the hidden host follows the PiP window instead of a collapsed size.
  const updateSize = () => {
    host.style.width = `${pipWindow?.width || video.videoWidth || 640}px`;
    host.style.height = `${pipWindow?.height || video.videoHeight || 360}px`;
  };
  updateSize();
  pipWindow?.addEventListener('resize', updateSize);
  document.body.append(host);
  const preserveState = typeof host.moveBefore === 'function' && video.isConnected;
  const resumePlayback = !video.paused;
  if (preserveState) host.moveBefore(video, null);
  else host.append(video);

  const dispose = () => {
    video.removeEventListener('leavepictureinpicture', dispose);
    pipWindow?.removeEventListener('resize', updateSize);
    unregisterCallVideo(video);
    retained.delete(track);
    track.detach(video);
    host.remove();
    // Remounted controls must now resolve their own video after browser closure.
    notifyChange();
  };
  retained.set(track, { video, dispose });
  video.addEventListener('leavepictureinpicture', dispose);
  if (!preserveState && resumePlayback) {
    void video.play().catch(() => {
      if (retained.get(track)?.video !== video) return;
      endCallVideo(track);
      toastError(null, m('voice.picture_in_picture_failed'));
    });
  }
}

/** Call ownership, not tile visibility, determines when a video stream is no longer usable. */
export function endCallVideo(track: Track): void {
  endedTracks.add(track);
  notifyChange();
  const active = typeof document === 'undefined' ? null : document.pictureInPictureElement;
  if (active && active instanceof HTMLVideoElement && videoTracks.get(active) === track) {
    void document.exitPictureInPicture().catch(() => {});
  }
  retained.get(track)?.dispose();
}
