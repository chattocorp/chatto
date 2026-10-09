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
const changes = new EventTarget();

/** Broadcast changes to stream ownership, retained video, or pending PiP requests. */
export function notifyCallVideoChange(): void {
  changes.dispatchEvent(new Event('change'));
}

/** Subscribe call controls to ownership changes; return a cleanup for the mounted observer. */
export function observeCallVideoChanges(onchange: () => void): () => void {
  changes.addEventListener('change', onchange);
  return () => changes.removeEventListener('change', onchange);
}

/** Return only the connected, call-owned track; ended streams cannot accept late PiP requests. */
export function activeCallVideoTrack(video: HTMLVideoElement): Track | undefined {
  const track = videoTracks.get(video);
  return track && !endedTracks.has(track) && video.isConnected ? track : undefined;
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
  notifyCallVideoChange();
}

/** Associate a tile video with its call-owned track without keeping either alive. */
export function registerCallVideo(track: Track, video: HTMLVideoElement): void {
  endedTracks.delete(track);
  videoTracks.set(video, track);
  video.addEventListener('enterpictureinpicture', rememberPictureInPictureWindow);
  video.addEventListener('leavepictureinpicture', forgetPictureInPictureWindow);
  notifyCallVideoChange();
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
    notifyCallVideoChange();
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
  notifyCallVideoChange();
  const active = typeof document === 'undefined' ? null : document.pictureInPictureElement;
  if (active && active instanceof HTMLVideoElement && videoTracks.get(active) === track) {
    void document.exitPictureInPicture().catch(() => {});
  }
  retained.get(track)?.dispose();
}
