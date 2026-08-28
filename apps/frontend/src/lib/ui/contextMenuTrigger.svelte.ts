import type { Attachment } from 'svelte/attachments';
import { on } from 'svelte/events';

const LONG_PRESS_MS = 500;
const LONG_PRESS_CANCEL_PX = 8;

export type ContextMenuTriggerDetails = {
	position: { x: number; y: number };
	presentation: 'auto' | 'sheet';
};

/**
 * Opens a context menu for a native context-menu gesture or a stationary touch long-press.
 * Movement cancels the pending touch gesture so normal sidebar scrolling remains native.
 */
export function contextMenuTrigger(
	onopen: (details: ContextMenuTriggerDetails) => void
): Attachment<HTMLElement> {
	return (node) => {
		let timer: number | null = null;
		let suppressClickTimer: number | null = null;
		let pointerId: number | null = null;
		let touchSequence = false;
		let startX = 0;
		let startY = 0;
		let suppressClick = false;

		function cancelLongPress(): void {
			if (timer !== null) window.clearTimeout(timer);
			timer = null;
			pointerId = null;
		}

		function armClickSuppression(): void {
			suppressClick = true;
			if (suppressClickTimer !== null) window.clearTimeout(suppressClickTimer);
			// Some browsers omit the synthetic click after a native long-press menu.
			// Do not let that suppress a later deliberate click.
			suppressClickTimer = window.setTimeout(() => {
				suppressClick = false;
				suppressClickTimer = null;
			}, 1000);
		}

		const cleanups = [
			on(node, 'contextmenu', (event) => {
				event.preventDefault();
				const fromTouch = touchSequence || suppressClick;
				if (fromTouch && suppressClick) return;
				cancelLongPress();
				if (fromTouch) armClickSuppression();
				onopen({
					position: { x: event.clientX, y: event.clientY },
					presentation: fromTouch ? 'sheet' : 'auto'
				});
			}),
			on(node, 'pointerdown', (event) => {
				if (event.pointerType !== 'touch' || !event.isPrimary) return;
				cancelLongPress();
				touchSequence = true;
				pointerId = event.pointerId;
				startX = event.clientX;
				startY = event.clientY;
				timer = window.setTimeout(() => {
					timer = null;
					pointerId = null;
					armClickSuppression();
					onopen({
						position: { x: startX, y: startY },
						presentation: 'sheet'
					});
				}, LONG_PRESS_MS);
			}),
			on(node, 'pointermove', (event) => {
				if (event.pointerId !== pointerId) return;
				if (
					Math.abs(event.clientX - startX) >= LONG_PRESS_CANCEL_PX ||
					Math.abs(event.clientY - startY) >= LONG_PRESS_CANCEL_PX
				) {
					cancelLongPress();
					touchSequence = false;
				}
			}),
			on(node, 'pointerup', () => {
				cancelLongPress();
				touchSequence = false;
			}),
			on(node, 'pointercancel', () => {
				cancelLongPress();
				touchSequence = false;
			}),
			on(node, 'click', (event) => {
				if (!suppressClick) return;
				suppressClick = false;
				if (suppressClickTimer !== null) window.clearTimeout(suppressClickTimer);
				suppressClickTimer = null;
				event.preventDefault();
				event.stopPropagation();
			}, { capture: true })
		];

		return () => {
			cancelLongPress();
			if (suppressClickTimer !== null) window.clearTimeout(suppressClickTimer);
			for (const cleanup of cleanups) cleanup();
		};
	};
}
