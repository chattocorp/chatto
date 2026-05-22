import { serverRegistry } from './server/registry.svelte';

/**
 * Tracks whether the user is "actively engaged" with the app, so callers
 * (currently the auto-refresh path in `UpdateNotifier`) can pick a safe
 * moment to take disruptive actions like reloading the page.
 *
 * "Safe to reload" means: the user is not typing into anything and is not
 * in a voice/video call. Extend the heuristics here — keep this the single
 * source of truth.
 *
 * Lifecycle: `isInputFocused` is driven by `<svelte:document>` listeners
 * in the root layout, not by this store. The store is pure state.
 */
class IdleState {
	/** True iff an editable element (input/textarea/contenteditable) has focus. */
	isInputFocused = $state(false);

	/** True iff the user is connected to any voice call on any registered server. */
	get isInAnyCall(): boolean {
		for (const server of serverRegistry.servers) {
			const store = serverRegistry.tryGetStore(server.id);
			if (store?.voiceCall.isInAnyCall) return true;
		}
		return false;
	}

	/**
	 * True iff it's safe to disruptively reload the page right now. The
	 * inverse of "user is actively engaged."
	 */
	get canSafelyReload(): boolean {
		return !this.isInputFocused && !this.isInAnyCall;
	}
}

/** Whether the given element is text-editable (input/textarea/contenteditable). */
export function isEditableElement(target: Element | null): boolean {
	if (!(target instanceof HTMLElement)) return false;
	const tag = target.tagName;
	if (tag === 'INPUT' || tag === 'TEXTAREA') return true;
	if (target.isContentEditable) return true;
	return false;
}

export const idleState = new IdleState();
