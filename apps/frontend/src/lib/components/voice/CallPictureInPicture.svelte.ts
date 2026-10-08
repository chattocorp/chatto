import { pictureInPictureVideo } from '$lib/state/callPictureInPicture';
import { m } from '$lib/i18n/messages';
import { toastError } from '$lib/utils/errorMessage';

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
  #video = $state.raw<HTMLVideoElement | null>(null);
  #videoGeneration = 0;

  /** Only cards with an observed media action expose PiP and fullscreen. */
  get hasMedia(): boolean {
    return this.#video !== null;
  }

  /** Observe a track's tile video, including a retained original video after a remount. */
  observeVideo = (video: HTMLVideoElement): (() => void) => {
    this.#video = video;
    const generation = ++this.#videoGeneration;
    this.supported =
      typeof video.requestPictureInPicture === 'function' && document.pictureInPictureEnabled;
    const update = () => {
      this.ready = video.readyState >= HTMLMediaElement.HAVE_METADATA && video.videoWidth > 0;
      this.active = document.pictureInPictureElement === pictureInPictureVideo(video);
    };
    const events = [
      'loadedmetadata',
      'loadeddata',
      'resize',
      'emptied',
      'enterpictureinpicture',
      'leavepictureinpicture'
    ];
    for (const event of events) video.addEventListener(event, update);
    document.addEventListener('enterpictureinpicture', update, true);
    document.addEventListener('leavepictureinpicture', update, true);
    update();
    return () => {
      for (const event of events) video.removeEventListener(event, update);
      document.removeEventListener('enterpictureinpicture', update, true);
      document.removeEventListener('leavepictureinpicture', update, true);
      if (generation !== this.#videoGeneration) return;
      ++this.#videoGeneration;
      this.#video = null;
      this.supported = false;
      this.ready = false;
      this.active = false;
    };
  };

  /** Toggle this card's video synchronously from a user gesture; reject duplicate requests. */
  togglePictureInPicture = async (event: MouseEvent): Promise<void> => {
    event.stopPropagation();
    const video = this.#video && pictureInPictureVideo(this.#video);
    if (!video || this.pending || !this.supported || (!this.ready && !this.active)) return;
    const generation = this.#videoGeneration;
    this.pending = true;
    try {
      if (document.pictureInPictureElement === video) {
        await document.exitPictureInPicture();
      } else {
        await video.requestPictureInPicture();
        if (generation !== this.#videoGeneration && document.pictureInPictureElement === video) {
          await document.exitPictureInPicture();
        }
      }
    } catch {
      if (generation === this.#videoGeneration)
        toastError(null, m('voice.picture_in_picture_failed'));
    } finally {
      this.pending = false;
    }
  };
}
