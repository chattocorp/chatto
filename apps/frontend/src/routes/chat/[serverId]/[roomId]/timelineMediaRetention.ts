import type { Attachment } from 'svelte/attachments';
import { TIMELINE_ITEM_KEY_ATTRIBUTE } from './timelineSelection';

/**
 * Report timeline rows with playing attachment media. Capture native events because
 * they do not bubble; this also observes the native video inside Vidstack.
 * The mounted timeline owns the listeners and media references; cleanup releases both.
 */
export function retainPlayingTimelineMedia(
  onChange: (keys: ReadonlySet<string>) => void
): Attachment<HTMLElement> {
  return (node) => {
    const playing = new Map<HTMLMediaElement, string>();
    let reported = new Set<string>();

    function report() {
      const keys = new Set(playing.values());
      if (keys.size === reported.size && [...keys].every((key) => reported.has(key))) return;
      reported = keys;
      onChange(keys);
    }

    function itemKey(media: HTMLMediaElement): string | null {
      if (
        !node.contains(media) ||
        media.hasAttribute('data-autoloop') ||
        !media.closest('[data-attachment-media]')
      ) {
        return null;
      }
      return (
        media
          .closest(`[${TIMELINE_ITEM_KEY_ATTRIBUTE}]`)
          ?.getAttribute(TIMELINE_ITEM_KEY_ATTRIBUTE) ?? null
      );
    }

    function handlePlay(event: Event) {
      if (!(event.target instanceof HTMLMediaElement)) return;
      const key = itemKey(event.target);
      if (!key) return;
      playing.set(event.target, key);
      report();
    }

    function handleStop(event: Event) {
      if (event.target instanceof HTMLMediaElement && playing.delete(event.target)) report();
    }

    const stopEvents = ['pause', 'ended', 'error', 'emptied'] as const;
    node.addEventListener('play', handlePlay, true);
    for (const type of stopEvents) node.addEventListener(type, handleStop, true);

    // Media can already be playing when the timeline attachment is installed.
    for (const media of node.querySelectorAll<HTMLMediaElement>('audio, video')) {
      const key = itemKey(media);
      if (key && !media.paused && !media.ended && !media.error) playing.set(media, key);
    }
    report();

    // Removing an attachment need not emit a pause event through the timeline.
    const observer = new MutationObserver(() => {
      for (const media of playing.keys()) {
        if (itemKey(media) !== playing.get(media)) playing.delete(media);
      }
      report();
    });
    observer.observe(node, { childList: true, subtree: true });

    return () => {
      node.removeEventListener('play', handlePlay, true);
      for (const type of stopEvents) node.removeEventListener(type, handleStop, true);
      observer.disconnect();
      playing.clear();
      report();
    };
  };
}
