import type { Attachment } from 'svelte/attachments';

/** Safari's presentation mode, which Vidstack uses for picture-in-picture where available. */
type WebKitVideoElement = HTMLVideoElement & { webkitPresentationMode?: string };

/**
 * Report whether media inside an element needs to stay mounted.
 *
 * Media is active while it plays, including buffering and seeking, and while a
 * video shows in picture-in-picture. A video in picture-in-picture stays active
 * after pause or playback end, so its window stays controllable when its timeline
 * row is offscreen. Removal of the element reports `false` if it was active.
 *
 * Listeners use the capture phase, so a wrapper such as Vidstack's
 * `<media-player>` also gets the non-bubbling events of its inner `<video>`.
 * They accept only events from the element itself or from a media element:
 * an `error` from a poster image or `<source>` does not stop playback.
 * Repeated events report only changes.
 *
 * @param onChange Receives `true` when the media becomes active and `false` when it is released.
 */
export function retainPlayback(onChange: (active: boolean) => void): Attachment<HTMLElement> {
  return (node) => {
    let playing = false;
    let pictureInPicture = false;
    let active = false;

    function update(next: { playing?: boolean; pictureInPicture?: boolean }) {
      ({ playing = playing, pictureInPicture = pictureInPicture } = next);
      if (playing || pictureInPicture) {
        if (!active) onChange((active = true));
      } else if (active) {
        onChange((active = false));
      }
    }

    const listeners: Record<string, (target: EventTarget) => void> = {
      play: () => update({ playing: true }),
      pause: () => update({ playing: false }),
      ended: () => update({ playing: false }),
      emptied: () => update({ playing: false }),
      error: () => update({ playing: false }),
      enterpictureinpicture: () => update({ pictureInPicture: true }),
      leavepictureinpicture: () => update({ pictureInPicture: false }),
      webkitpresentationmodechanged: (target) => {
        if (!(target instanceof HTMLVideoElement)) return;
        const mode = (target as WebKitVideoElement).webkitPresentationMode;
        update({ pictureInPicture: mode === 'picture-in-picture' });
      }
    };
    const controller = new AbortController();
    for (const [type, listener] of Object.entries(listeners)) {
      node.addEventListener(
        type,
        ({ target }) => {
          if (target === node || target instanceof HTMLMediaElement) listener(target);
        },
        { capture: true, signal: controller.signal }
      );
    }

    return () => {
      controller.abort();
      if (active) onChange(false);
    };
  };
}
