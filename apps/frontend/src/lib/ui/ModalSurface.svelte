<!-- @component
Internal native modal owner shared by task dialogs and bottom sheets.
Presentation changes keep the same native dialog and content mounted. Only
the handle claims drag gestures; content retains native scrolling.
-->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import { m } from '$lib/i18n/messages';
	import { panGesture } from '$lib/hooks/panGesture.svelte';
	import { shouldAutoFocus } from '$lib/utils/shouldAutoFocus';

	let {
		visible = $bindable(false),
		sheet = false,
		class: className,
		labelledBy,
		describedBy,
		ariaLabel,
		autofocusContent = false,
		onclose,
		onkeydown,
		children
	}: {
		visible?: boolean;
		sheet?: boolean;
		class?: string;
		labelledBy?: string;
		describedBy?: string;
		ariaLabel?: string;
		autofocusContent?: boolean;
		onclose?: () => void;
		onkeydown?: (event: KeyboardEvent) => void;
		children: Snippet<[close: () => void, handle: Snippet]>;
	} = $props();

	let dialogEl: HTMLDialogElement | undefined;
	let closing = $state(false);
	let dragging = $state(false);
	let dragOffset = $state(0);
	// A claimed drag can still produce a browser click after pointerup.
	let handleDragged = false;
	let previousFocus: Element | null = null;
	let pressStartedInside = true;
	let editableFocusPending = false;
	let closeTimer: ReturnType<typeof setTimeout> | undefined;

	function restoreFocus() {
		if (previousFocus instanceof HTMLElement && previousFocus.isConnected) {
			previousFocus.focus({ preventScroll: true });
		}
	}

	function lifetime(node: HTMLDialogElement) {
		dialogEl = node;
		return () => {
			clearTimeout(closeTimer);
			node.close();
			restoreFocus();
		};
	}

	function syncVisibility(node: HTMLDialogElement) {
		if (visible && !node.open) {
			closing = false;
			dragOffset = 0;
			pressStartedInside = true;
			previousFocus = document.activeElement;
			node.showModal();
			if (autofocusContent && shouldAutoFocus()) {
				queueMicrotask(() => {
					if (!node.open) return;
					const fields =
						'input:not([type="hidden"]):not([disabled]),textarea:not([disabled]),select:not([disabled])';
					if (
						document.activeElement instanceof HTMLElement &&
						node.contains(document.activeElement) &&
						document.activeElement.matches(fields)
					)
						return;
					(
						node.querySelector<HTMLElement>(fields) ??
						node.querySelector<HTMLElement>(
							'button[type="submit"]:not([disabled]),button[data-dialog-default]:not([disabled])'
						)
					)?.focus();
				});
			}
		} else if (!visible && node.open && !closing) node.close();
	}

	/** Top-layer dialogs do not inherit body height fixes for virtual keyboards. */
	function viewport(node: HTMLDialogElement) {
		const vv = window.visualViewport;
		if (!vv) return;
		function update() {
			node.style.setProperty('--modal-viewport-height', `${vv!.height}px`);
			node.style.setProperty(
				'--modal-viewport-bottom',
				`${Math.max(0, window.innerHeight - vv!.height - vv!.offsetTop)}px`
			);
		}
		update();
		vv.addEventListener('resize', update);
		vv.addEventListener('scroll', update);
		return () => {
			vv.removeEventListener('resize', update);
			vv.removeEventListener('scroll', update);
		};
	}

	function close() {
		if (!dialogEl?.open || closing) return;
		closing = true;
		// Also close when an animation is disabled or interrupted.
		closeTimer = setTimeout(() => dialogEl?.close(), sheet ? 350 : 100);
	}

	function nativeClose() {
		clearTimeout(closeTimer);
		visible = false;
		closing = false;
		dragging = false;
		dragOffset = 0;
		restoreFocus();
		onclose?.();
	}

	function isEditable(target: EventTarget | null) {
		return (
			target instanceof Element &&
			dialogEl?.contains(target) &&
			!!target.closest('input,textarea,[contenteditable]:not([contenteditable="false"])')
		);
	}

	function pressStart(event: PointerEvent | TouchEvent) {
		pressStartedInside = event.target !== dialogEl;
		editableFocusPending = !!isEditable(event.target);
		// Keyboard movement can retarget a sheet's later click.
		if (sheet && !pressStartedInside) close();
	}
</script>

{#snippet handle()}
	{#if sheet}
		<button
			use:panGesture={{
				axis: 'y',
				enabled: () => !closing,
				shouldClaim: (dy) => dy > 0,
					onStart: () => {
						handleDragged = true;
						dragging = true;
				},
				onUpdate: (dy) => {
					dragOffset = Math.max(0, dy);
				},
				onEnd: (dy, velocity) => {
					dragging = false;
					const height = dialogEl?.offsetHeight ?? 0;
					if ((height > 0 && dy > height * 0.5) || velocity > 0.5) {
						dragOffset = height;
						close();
					} else dragOffset = 0;
				},
				onCancel: () => {
					dragging = false;
					dragOffset = 0;
				}
			}}
			type="button"
			class="flex min-h-8 w-full shrink-0 cursor-pointer touch-none items-center justify-center"
				onpointerdown={() => (handleDragged = false)}
				ontouchstart={() => (handleDragged = false)}
				onclick={(event) => {
					if (event.detail === 0 || !handleDragged) close();
				}}
			aria-label={m('ui.close')}
		>
			<span class="h-1 w-10 rounded-full bg-muted/40"></span>
		</button>
	{/if}
{/snippet}

<dialog
	{@attach lifetime}
	{@attach syncVisibility}
	{@attach viewport}
	onclose={nativeClose}
	{onkeydown}
	oncancel={(event) => {
		event.preventDefault();
		if (
			sheet &&
			(editableFocusPending || (isEditable(document.activeElement) && !shouldAutoFocus()))
		)
			return;
		close();
	}}
	onpointerdown={pressStart}
	ontouchstart={pressStart}
	onfocusin={() => {
		editableFocusPending = false;
	}}
	onclick={(event) => {
		if (sheet || event.detail === 0 || pressStartedInside) return;
		const rect = dialogEl?.firstElementChild?.getBoundingClientRect();
		if (
			rect &&
			(event.clientX < rect.left ||
				event.clientX > rect.right ||
				event.clientY < rect.top ||
				event.clientY > rect.bottom)
		)
			close();
	}}
	onanimationend={(event) => {
		if (
			closing &&
			event.target === dialogEl &&
			!event.pseudoElement &&
			event.animationName.endsWith('slide-down')
		)
			dialogEl?.close();
	}}
	aria-labelledby={labelledBy}
	aria-describedby={describedBy}
	aria-label={ariaLabel}
	class={[
		'modal-surface bg-transparent p-0 backdrop:bg-black/50',
		sheet ? 'bottom-sheet' : 'm-auto',
		className
	]}
	class:closing
	class:dragging
	style:--modal-drag-offset={`${dragOffset}px`}
>
	{#if visible || closing}
		{@render children(close, handle)}
	{/if}
</dialog>

<style>
	.modal-surface {
		border: 0;
	}
	.bottom-sheet {
		--modal-available-height: calc(var(--modal-viewport-height, 100dvh) - env(safe-area-inset-top) - 8px);
		margin: 0;
		margin-top: auto;
		bottom: var(--modal-viewport-bottom, 0px);
		width: 100%;
		max-width: 100%;
		max-height: var(--modal-available-height);
	}
	.bottom-sheet > :global(*) {
		transform: translateY(var(--modal-drag-offset));
		transition: transform var(--motion-duration-pane) var(--ease-out-expo);
	}
	.dragging > :global(*) {
		transition: none;
	}
	.modal-surface[open] {
		animation: fade-in var(--motion-duration-overlay-enter) var(--motion-easing-overlay-enter);
	}
	.modal-surface[open]::backdrop {
		animation: backdrop-in var(--motion-duration-overlay-enter) ease-out;
	}
	.modal-surface[open].closing {
		animation: fade-out 100ms ease-in forwards;
	}
	.modal-surface[open].closing::backdrop {
		animation: backdrop-out 100ms ease-in forwards;
	}
	.bottom-sheet[open] {
		animation: slide-up var(--motion-duration-pane) var(--ease-out-expo);
	}
	.bottom-sheet[open].closing {
		animation: slide-down var(--motion-duration-pane) var(--ease-out-expo) forwards;
	}
	@keyframes fade-in {
		from {
			opacity: 0;
			transform: scale(0.95);
		}
		to {
			opacity: 1;
			transform: scale(1);
		}
	}
	@keyframes fade-out {
		to {
			opacity: 0;
			transform: scale(0.95);
		}
	}
	@keyframes slide-up {
		from {
			transform: translateY(100%);
		}
		to {
			transform: translateY(0);
		}
	}
	@keyframes slide-down {
		to {
			transform: translateY(100%);
		}
	}
	@keyframes backdrop-in {
		from {
			opacity: 0;
		}
		to {
			opacity: 1;
		}
	}
	@keyframes backdrop-out {
		to {
			opacity: 0;
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.modal-surface[open],
		.modal-surface[open].closing,
		.modal-surface[open]::backdrop,
		.modal-surface[open].closing::backdrop {
			animation-duration: 0.01ms;
		}
		.bottom-sheet > :global(*) {
			transition: none;
		}
	}
</style>
