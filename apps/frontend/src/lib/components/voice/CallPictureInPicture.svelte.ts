import {
  activeCallVideoTrack,
  notifyCallVideoChange,
  observeCallVideoChanges,
  pictureInPictureVideo
} from '$lib/state/callPictureInPicture';
import { m } from '$lib/i18n/messages';
import { toastError } from '$lib/utils/errorMessage';

/** Request state stays in the lazy call UI; the shared owner keeps only stream lifetimes. */
const pendingVideos = new WeakSet<HTMLVideoElement>();

/** Browser state shared by a call video's header button and user-menu entry. */
type CallPictureInPictureStatus = {
  supported: boolean;
  ready: boolean;
  active: boolean;
  pending: boolean;
  /** False when the selected tile or its call-owned stream has ended. */
  available: boolean;
};

/** Read the retained video, so controls follow PiP across tile remounts. */
function pictureInPictureStatus(video: HTMLVideoElement): CallPictureInPictureStatus {
  const element = pictureInPictureVideo(video);
  const track = activeCallVideoTrack(element);
  return {
    supported:
      typeof element.requestPictureInPicture === 'function' &&
      document.pictureInPictureEnabled &&
      !element.disablePictureInPicture,
    ready: element.readyState >= HTMLMediaElement.HAVE_METADATA && element.videoWidth > 0,
    active: document.pictureInPictureElement === element,
    pending: pendingVideos.has(element),
    available: !!track
  };
}

/** Observe media readiness, browser PiP changes, and shared requests until cleanup. */
function observeCallPictureInPicture(
  video: HTMLVideoElement,
  onchange: (status: CallPictureInPictureStatus) => void
): () => void {
  const update = () => onchange(pictureInPictureStatus(video));
  const events = ['loadedmetadata', 'loadeddata', 'resize', 'emptied'];
  for (const event of events) video.addEventListener(event, update);
  document.addEventListener('enterpictureinpicture', update, true);
  document.addEventListener('leavepictureinpicture', update, true);
  const cleanup = observeCallVideoChanges(update);
  update();
  return () => {
    for (const event of events) video.removeEventListener(event, update);
    document.removeEventListener('enterpictureinpicture', update, true);
    document.removeEventListener('leavepictureinpicture', update, true);
    cleanup();
  };
}

/**
 * Toggle from a user click, without an async step before the browser request.
 * Requests belong to the video, so closing a menu does not cancel them.
 * Return true after success. Rejections show the call's localized error;
 * removed tiles and ended streams discard late results without an error.
 */
async function toggleCallPictureInPicture(video: HTMLVideoElement): Promise<boolean> {
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
  const track = activeCallVideoTrack(element)!;
  const available = () => activeCallVideoTrack(element) === track;
  pendingVideos.add(element);
  notifyCallVideoChange();
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
    notifyCallVideoChange();
  }
}

/** Owns one card's PiP state independently of its toolbar and menu presentation. */
export class CallPictureInPicture {
  /** Whether the browser allows PiP requests from the page. */
  supported = $state(false);
  /** Whether this video has metadata and a nonzero width. */
  ready = $state(false);
  /** Whether this track, including its retained original video, is in PiP. */
  active = $state(false);
  /** A PiP request is still running; both presentations must reject another request. */
  pending = $state(false);
  /** Whether the selected video still belongs to an active call stream. */
  available = $state(false);
  #video = $state.raw<HTMLVideoElement | null>(null);
  #videoGeneration = 0;

  /** Whether this card observes a video, including a secondary stage tile. */
  get hasMedia(): boolean {
    return this.#video !== null;
  }

  /** Observe a track's tile video, including a retained original video after a remount. */
  observeVideo = (video: HTMLVideoElement): (() => void) => {
    this.#video = video;
    const generation = ++this.#videoGeneration;
    const cleanup = observeCallPictureInPicture(video, (status) => {
      this.supported = status.supported;
      this.ready = status.ready;
      this.active = status.active;
      this.pending = status.pending;
      this.available = status.available;
    });
    return () => {
      cleanup();
      if (generation !== this.#videoGeneration) return;
      ++this.#videoGeneration;
      this.#video = null;
      this.supported = false;
      this.ready = false;
      this.active = false;
      this.pending = false;
      this.available = false;
    };
  };

  /** Toggle this card's video synchronously from a user gesture; reject duplicate requests. */
  togglePictureInPicture = (event: MouseEvent): Promise<boolean> => {
    event.stopPropagation();
    return this.#video ? toggleCallPictureInPicture(this.#video) : Promise.resolve(false);
  };
}
