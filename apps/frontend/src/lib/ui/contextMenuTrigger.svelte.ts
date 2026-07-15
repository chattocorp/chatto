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
		let pointerId: number | null = null;
		let startX = 0;
		let startY = 0;
		let suppressClick = false;

		function cancelLongPress(): void {
			if (timer !== null) window.clearTimeout(timer);
			timer = null;
			pointerId = null;
		}

		const cleanups = [
			on(node, 'contextmenu', (event) => {
				event.preventDefault();
				cancelLongPress();
				onopen({
					position: { x: event.clientX, y: event.clientY },
					presentation: 'auto'
				});
			}),
			on(node, 'pointerdown', (event) => {
				if (event.pointerType !== 'touch' || !event.isPrimary) return;
				cancelLongPress();
				pointerId = event.pointerId;
				startX = event.clientX;
				startY = event.clientY;
				timer = window.setTimeout(() => {
					timer = null;
					pointerId = null;
					suppressClick = true;
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
				}
			}),
			on(node, 'pointerup', cancelLongPress),
			on(node, 'pointercancel', cancelLongPress),
			on(node, 'click', (event) => {
				if (!suppressClick) return;
				suppressClick = false;
				event.preventDefault();
				event.stopPropagation();
			})
		];

		return () => {
			cancelLongPress();
			for (const cleanup of cleanups) cleanup();
		};
	};
}
