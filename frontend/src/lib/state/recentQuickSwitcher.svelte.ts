/**
 * Recent Quick Switcher destinations.
 *
 * Tracks the last destinations the user navigated to via the Quick Switcher.
 * Stored as an ordered list of URL paths (most recent first) in localStorage.
 * URLs already encode the instance segment, so no per-instance namespacing needed.
 */

const STORAGE_KEY = 'chatto:quickSwitcherRecents';
const MAX_RECENTS = 15;

export class RecentQuickSwitcherState {
	private recents = $state<string[]>([]);

	constructor() {
		if (typeof window !== 'undefined') {
			try {
				const stored = localStorage.getItem(STORAGE_KEY);
				if (stored) {
					const parsed = JSON.parse(stored);
					if (Array.isArray(parsed)) {
						this.recents = parsed.filter((e): e is string => typeof e === 'string');
					}
				}
			} catch {
				// Ignore corrupt localStorage
			}
		}
	}

	/** Ordered list of recent destination URLs, most recent first. */
	get urls(): readonly string[] {
		return this.recents;
	}

	/** Record a destination URL as the most recently used. */
	record(url: string) {
		const filtered = this.recents.filter((u) => u !== url);
		this.recents = [url, ...filtered].slice(0, MAX_RECENTS);
		this.persist();
	}

	/** Returns the recency index (0 = most recent) or -1 if not recent. */
	indexOf(url: string): number {
		return this.recents.indexOf(url);
	}

	private persist() {
		try {
			localStorage.setItem(STORAGE_KEY, JSON.stringify(this.recents));
		} catch {
			// localStorage full or unavailable
		}
	}
}

export const recentQuickSwitcher = new RecentQuickSwitcherState();
