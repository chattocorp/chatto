<!-- @component
Standard task shell: a bottom sheet below md and a framed dialog above it.
Declare primaryAction, secondaryActions, and dismissAction once. The shell
owns their layout and reading order. Body state survives viewport changes.
The footer and footerDetails escape hatches are for specialized viewer controls
and cannot be combined with semantic actions.
-->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import { MediaQuery } from 'svelte/reactivity';
	import { m } from '$lib/i18n/messages';
	import ModalSurface from './ModalSurface.svelte';
	/** Task actions and specialized viewer footers are mutually exclusive. */
	type ActionContent =
		| {
				/** Main action; first on mobile, last on desktop. Does not imply Enter activation. */
				primaryAction?: Snippet;
				/** Alternative actions in their intended reading order. */
				secondaryActions?: Snippet;
				/** Cancel or Close action; last on mobile, first on desktop. */
				dismissAction?: Snippet;
				footer?: never;
				footerDetails?: never;
			}
		| {
				primaryAction?: never;
				secondaryActions?: never;
				dismissAction?: never;
				/** Specialized viewer controls. Task dialogs use semantic action snippets. */
				footer: Snippet;
				/** File or task details associated with specialized viewer controls. */
				footerDetails?: Snippet;
			};
	let {
		children: body,
		primaryAction,
		secondaryActions,
		dismissAction,
		footer,
		footerDetails,
		mediaViewer = false,
		visible = $bindable(false),
		title,
		size = 'md',
		describedBy,
		onclose
	}: {
		visible?: boolean;
		title?: string;
		size?: 'sm' | 'md' | 'lg' | 'xl';
		/** Accessible description element ID. */
		describedBy?: string;
		children: Snippet;
		/** Full-screen media area on small viewports. */
		mediaViewer?: boolean;
		onclose?: () => void;
	} & ActionContent = $props();
	const narrow = new MediaQuery('(width < 768px)');
	const sheet = $derived(narrow.current && !mediaViewer);
	const id = $props.id();
	const titleId = `${id}-title`;
	const widths = { sm: '400px', md: '600px', lg: '800px', xl: 'min(90vw, 1440px)' };
	const actions = $derived(
		(sheet
			? [primaryAction, secondaryActions, dismissAction]
			: [dismissAction, secondaryActions, primaryAction]
		).filter((action) => action !== undefined)
	);
	/** Moving a keyed action can clear native focus even though its DOM node survives. */
	function preserveActionFocus(node: HTMLElement) {
		let focused: HTMLElement | null = null;
		function remember(event: FocusEvent) {
			focused = event.target instanceof HTMLElement ? event.target : null;
		}
		function forget(event: FocusEvent) {
			if (event.relatedTarget) focused = null;
		}
		const observer = new MutationObserver(() => {
			if (
				focused &&
				node.contains(focused) &&
				document.activeElement === document.body &&
				node.closest('dialog')?.open
			) {
				focused.focus({ preventScroll: true });
			}
		});
		node.addEventListener('focusin', remember);
		node.addEventListener('focusout', forget);
		observer.observe(node, { childList: true });
		return () => {
			observer.disconnect();
			node.removeEventListener('focusin', remember);
			node.removeEventListener('focusout', forget);
		};
	}
	/** Reserve the measured header and action height, including wrapped labels. */
	function measureChrome(node: HTMLElement) {
		const header = node.querySelector(':scope > header')!;
		const footer = node.querySelector(':scope > footer');
		function outerHeight(element: Element | null) {
			if (!element) return 0;
			const style = getComputedStyle(element);
			return (
				element.getBoundingClientRect().height +
				parseFloat(style.marginTop) +
				parseFloat(style.marginBottom)
			);
		}
		let frame = 0;
		function update() {
			const style = getComputedStyle(node);
			const frameStyle = getComputedStyle(node.parentElement!);
			const handle = node.previousElementSibling;
			const height =
				outerHeight(header) +
				outerHeight(footer) +
				outerHeight(handle) +
				parseFloat(style.paddingTop) +
				parseFloat(style.paddingBottom) +
				parseFloat(frameStyle.paddingTop) +
				parseFloat(frameStyle.paddingBottom) +
				parseFloat(frameStyle.borderTopWidth) +
				parseFloat(frameStyle.borderBottomWidth);
			node.style.setProperty('--dialog-chrome-height', `${height}px`);
		}
		const observer = new ResizeObserver(() => {
			cancelAnimationFrame(frame);
			frame = requestAnimationFrame(update);
		});
		observer.observe(node);
		observer.observe(header);
		if (footer) observer.observe(footer);
		return () => {
			observer.disconnect();
			cancelAnimationFrame(frame);
		};
	}
	function handleKeydown(event: KeyboardEvent) {
		if (event.key !== 'Enter' || event.defaultPrevented || event.isComposing || event.repeat)
			return;
		const target = event.target;
		if (
			target instanceof Element &&
			target.closest('form,button,a,textarea,select,[role="button"],[contenteditable="true"]')
		)
			return;
		const action = (event.currentTarget as HTMLElement).querySelector<HTMLButtonElement>(
			'button[data-dialog-default]:not([disabled])'
		);
		if (action) {
			event.preventDefault();
			action.click();
		}
	}
</script>

<ModalSurface
	bind:visible
	{sheet}
	labelledBy={title ? titleId : undefined}
	{describedBy}
	{onclose}
	autofocusContent
	onkeydown={handleKeydown}
	class={sheet
		? ''
		: mediaViewer
			? 'h-dvh max-h-dvh w-dvw max-w-dvw md:h-fit md:w-fit md:max-w-[calc(100vw-2rem)]'
			: 'w-fit max-w-[calc(100vw-2rem)]'}
>
	{#snippet children(close, handle)}
		<div
			class={[
				'dialog-frame flex max-w-full flex-col overflow-hidden bg-surface shadow-xl',
				sheet
					? 'task-sheet sheet-frame w-full'
					: mediaViewer
						? 'h-dvh max-h-dvh w-full md:h-[85dvh] md:max-h-[85dvh] md:w-max md:rounded-lg md:border md:border-text/10 md:floating-frame md:p-2'
						: 'max-h-[78vh] w-max rounded-lg border border-text/10 floating-frame p-2'
			]}
			style:--dialog-baseline-width={widths[size]}
		>
			{@render handle()}
			<div
				{@attach sheet ? measureChrome : undefined}
				class={[
					'dialog-work-plane flex min-h-0 max-w-full min-w-full flex-1 flex-col overflow-hidden',
					sheet
						? 'sheet-content w-full rounded-md bg-background floating-inset p-3'
						: 'w-max bg-background p-3',
					!sheet && (mediaViewer ? 'md:rounded-md md:floating-inset' : 'rounded-md floating-inset'),
					mediaViewer &&
						'ps-[max(0.75rem,env(safe-area-inset-left))] pe-[max(0.75rem,env(safe-area-inset-right))] pt-[max(0.75rem,env(safe-area-inset-top))] pb-[max(0.75rem,env(safe-area-inset-bottom))]'
				]}
			>
				<header
					class={[
						'flex w-0 min-w-full shrink-0 items-start justify-between gap-3',
						title ? 'mb-4' : 'mb-2'
					]}
				>
					{#if title}
						<h2
							id={titleId}
							class={[
								'min-w-0 text-xl font-semibold text-balance wrap-anywhere text-text-top',
								mediaViewer && 'line-clamp-2'
							]}
						>
							<bdi>{title}</bdi>
						</h2>
					{:else}<span></span>{/if}
					<button
						type="button"
						onclick={close}
						class="-m-2 icon-action shrink-0"
						aria-label={m('ui.close')}
					>
						<span class="iconify icon-[uil--times] text-xl"></span>
					</button>
				</header>
				<div
					class={[
						'dialog-body min-h-0 w-0 min-w-full text-text',
						mediaViewer
							? 'flex flex-1 flex-col overflow-hidden'
							: 'overflow-y-auto overscroll-contain'
					]}
				>
					{@render body()}
				</div>
				{#if actions.length}
					<footer
						{@attach preserveActionFocus}
						class={['dialog-actions', sheet && 'dialog-actions-sheet']}
					>
						{#each actions as action (action)}{@render action()}{/each}
					</footer>
				{:else if footer}
					<footer
						class={footerDetails
							? 'mt-3 flex min-w-0 shrink-0 items-center justify-between gap-4'
							: 'dialog-actions'}
					>
						{#if footerDetails}<div class="min-w-0 flex-1">{@render footerDetails()}</div>{/if}
						{@render footer()}
					</footer>
				{/if}
			</div>
		</div>
	{/snippet}
</ModalSurface>

<style>
	.dialog-frame {
		min-width: min(var(--dialog-baseline-width), calc(100vw - 2rem));
	}
	.task-sheet {
		min-width: 100%;
	}
	.sheet-content {
		/* Chrome can scroll when it cannot fit alongside a usable body region. */
		overflow-y: auto;
		overscroll-behavior: contain;
	}
	.sheet-content > .dialog-body {
		flex-shrink: 0;
		max-height: max(
			8rem,
			calc(var(--modal-available-height) - var(--dialog-chrome-height, 24rem))
		);
	}
</style>
