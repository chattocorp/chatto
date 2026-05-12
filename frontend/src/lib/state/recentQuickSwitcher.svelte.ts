/**
 * Recent Quick Switcher destinations.
 *
 * Tracks the last destinations the user navigated to via the Quick Switcher.
 * Stored as an ordered list of URL paths (most recent first) in localStorage.
 * URLs already encode the instance segment, so no per-instance namespacing needed.
 */

import { Codecs, globalSlot } from '$lib/storage/slot';

const MAX_RECENTS = 15;

const slot = globalSlot(
  'quickSwitcherRecents',
  [] as string[],
  Codecs.json<string[]>((v): v is string[] => Array.isArray(v))
);

export class RecentQuickSwitcherState {
  // Filter on read: the codec only validates that we have an array, so
  // individual corrupt entries are dropped here without invalidating the
  // entire payload (matches the original loader's behaviour).
  private recents = $state<string[]>(
    slot.get().filter((e): e is string => typeof e === 'string')
  );

  /** Ordered list of recent destination URLs, most recent first. */
  get urls(): readonly string[] {
    return this.recents;
  }

  /** Record a destination URL as the most recently used. */
  record(url: string) {
    const filtered = this.recents.filter((u) => u !== url);
    this.recents = [url, ...filtered].slice(0, MAX_RECENTS);
    slot.set(this.recents);
  }

  /** Returns the recency index (0 = most recent) or -1 if not recent. */
  indexOf(url: string): number {
    return this.recents.indexOf(url);
  }
}

export const recentQuickSwitcher = new RecentQuickSwitcherState();
