import {
  observeCallPictureInPicture,
  toggleCallPictureInPicture
} from '$lib/state/callPictureInPicture';

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
