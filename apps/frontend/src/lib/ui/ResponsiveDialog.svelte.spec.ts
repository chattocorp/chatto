import '../../app.css';
import { cdp, page, userEvent } from 'vitest/browser';
import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { tick } from 'svelte';
import ResponsiveDialogHarness from './ResponsiveDialogHarness.svelte';
import BottomSheet from './BottomSheet.svelte';
import { testSnippet } from '$lib/test-utils';

beforeEach(async () => {
	await cdp().send('Emulation.setTouchEmulationEnabled', { enabled: true });
});

afterEach(async () => {
	await cdp().send('Emulation.setTouchEmulationEnabled', { enabled: false });
	vi.restoreAllMocks();
	document.documentElement.style.fontSize = '';
	document.documentElement.dir = '';
});

describe('responsive task dialogs', () => {
	it.each([320, 390, 767, 768])('keeps a centred dialog usable with a mouse at %i px', async (width) => {
		await cdp().send('Emulation.setTouchEmulationEnabled', { enabled: false });
		await page.viewport(width, 844);
		const { container } = render(ResponsiveDialogHarness, { longBody: true });
		const dialog = container.querySelector('dialog')!;
		expect(dialog.classList.contains('bottom-sheet')).toBe(false);
		const frame = dialog.querySelector('.dialog-frame')!;
		expect(getComputedStyle(frame).borderRadius).not.toBe('0px');
		expect(dialog.getBoundingClientRect().width).toBeLessThanOrEqual(width - 32);
		expect(dialog.scrollWidth).toBeLessThanOrEqual(dialog.clientWidth);
		await expect.element(container.querySelector<HTMLButtonElement>('footer button')!).toBeVisible();
		for (const label of dialog.querySelectorAll<HTMLElement>('footer .button-content')) {
			expect(label.scrollWidth).toBeLessThanOrEqual(label.clientWidth);
		}
	});

	it('preserves the draft and focus when touch capability changes', async () => {
		await page.viewport(390, 844);
		const { container } = render(ResponsiveDialogHarness);
		const dialog = container.querySelector('dialog')!;
		const field = dialog.querySelector('textarea')!;
		field.value = 'Keep this draft';
		field.focus();
		await cdp().send('Emulation.setTouchEmulationEnabled', { enabled: false });
		await expect.poll(() => dialog.classList.contains('bottom-sheet')).toBe(false);
		expect(dialog.querySelector('textarea')).toBe(field);
		expect(field.value).toBe('Keep this draft');
		expect(document.activeElement).toBe(field);
		await cdp().send('Emulation.setTouchEmulationEnabled', { enabled: true });
		await expect.poll(() => dialog.classList.contains('bottom-sheet')).toBe(true);
		expect(field.value).toBe('Keep this draft');
		expect(document.activeElement).toBe(field);
	});

	it('uses the same frame surround as context-menu sheets', async () => {
		await page.viewport(390, 844);
		function surround(frame: Element, content: Element) {
			const outer = frame.getBoundingClientRect();
			const inner = content.getBoundingClientRect();
			return [inner.left - outer.left, outer.right - inner.right, outer.bottom - inner.bottom]
				.map((distance) => Math.round(distance));
		}
		const task = render(ResponsiveDialogHarness);
		const taskSurround = surround(
			task.container.querySelector('.sheet-frame')!,
			task.container.querySelector('.dialog-work-plane')!
		);
		await task.unmount();
		const menu = render(BottomSheet, {
			visible: true,
			children: testSnippet('<div class="menu-section">Menu actions</div>')
		});
		const menuSurround = surround(
			menu.container.querySelector('.sheet-frame')!,
			menu.container.querySelector('.menu-section')!
		);
		// The side measurements include the shared one-pixel outer border.
		expect(taskSurround).toEqual([17, 17, 16]);
		expect(taskSurround).toEqual(menuSurround);
	});

	it.each([320, 390])('shows full-width, complete actions at %i px', async (width) => {
		await page.viewport(width, 844);
		const { container } = render(ResponsiveDialogHarness, { loading: true });
		const dialog = container.querySelector('dialog')!;
		const footer = dialog.querySelector('footer')!;
		const buttons = [...footer.querySelectorAll('button')];
		expect(buttons.map((button) => button.textContent?.trim())).toEqual([
			'Deine Nachricht wird im vorherigen Diskussionsthread gesendet',
			'Als neue Nachricht senden',
			'Abbrechen'
		]);
		expect(dialog.offsetWidth).toBe(width);
		for (const button of buttons) {
			expect(button.offsetWidth).toBe(footer.clientWidth);
			expect(button.offsetHeight).toBeGreaterThanOrEqual(48);
			const label = button.querySelector('.button-content')!;
			expect(label.scrollWidth).toBeLessThanOrEqual(label.clientWidth);
		}
	});

	it('reorders mounted actions at md without replacing the form draft or focus', async () => {
		await page.viewport(390, 844);
		const { container } = render(ResponsiveDialogHarness);
		const dialog = container.querySelector('dialog')!;
		const field = dialog.querySelector('textarea')!;
		const primary = dialog.querySelector('[data-dialog-default]')!;
		field.value = 'A draft that must survive resizing';
		field.focus();
		await page.viewport(768, 844);
		await expect.poll(() => dialog.classList.contains('bottom-sheet')).toBe(false);
		expect(container.querySelector('dialog')).toBe(dialog);
		expect(dialog.querySelector('textarea')).toBe(field);
		expect(field.value).toBe('A draft that must survive resizing');
		expect(document.activeElement).toBe(field);
		expect(dialog.querySelector('[data-dialog-default]')).toBe(primary);
		expect(
			[...dialog.querySelectorAll('footer button')].map((button) => button.textContent?.trim())
		).toEqual(['Abbrechen', 'Als neue Nachricht senden', 'Im Thread fortfahren']);
		await page.viewport(390, 844);
		await expect.poll(() => dialog.querySelector('footer')!.firstElementChild).toBe(primary);
		expect(document.activeElement).toBe(field);
	});

	it('keeps actions visible while long body content scrolls', async () => {
		await page.viewport(390, 844);
		const { container } = render(ResponsiveDialogHarness, { longBody: true });
		const content = container.querySelector<HTMLElement>('.sheet-content')!;
		const body = container.querySelector<HTMLElement>('.dialog-body')!;
		await expect.poll(() => body.scrollHeight > body.clientHeight).toBe(true);
		await expect.poll(() => content.scrollHeight <= content.clientHeight + 1).toBe(true);
		const footer = container.querySelector('footer')!;
		await expect.poll(() => footer.getBoundingClientRect().bottom).toBeLessThanOrEqual(844);
	});

	it('preserves focus on an action when its reading order changes', async () => {
		await page.viewport(390, 844);
		const { container } = render(ResponsiveDialogHarness);
		const primary = container.querySelector<HTMLButtonElement>('[data-dialog-default]')!;
		await tick();
		primary.focus();
		await page.viewport(768, 844);
		await expect.poll(() => container.querySelector('footer')!.lastElementChild).toBe(primary);
		expect(document.activeElement).toBe(primary);
	});

	it('keeps all controls reachable in short RTL sheets with enlarged text', async () => {
		await page.viewport(568, 320);
		document.documentElement.style.fontSize = '32px';
		document.documentElement.dir = 'rtl';
		const { container } = render(ResponsiveDialogHarness, { longBody: true });
		const content = container.querySelector<HTMLElement>('.sheet-content')!;
		await expect.poll(() => content.scrollHeight > content.clientHeight).toBe(true);
		content.scrollTop = content.scrollHeight;
		const cancel = container.querySelector('footer')!.lastElementChild!;
		await expect.poll(() => cancel.getBoundingClientRect().bottom <= 320).toBe(true);
		expect(content.scrollWidth).toBeLessThanOrEqual(content.clientWidth);
	});

	it('tracks a shortened visual viewport and restores focus on unmount', async () => {
		await page.viewport(390, 844);
		const viewport = Object.assign(new EventTarget(), { height: 844, offsetTop: 0 });
		vi.spyOn(window, 'visualViewport', 'get').mockReturnValue(viewport as VisualViewport);
		const trigger = document.createElement('button');
		document.body.append(trigger);
		trigger.focus();
		const { container, unmount } = render(ResponsiveDialogHarness);
		const dialog = container.querySelector('dialog')!;
		viewport.height = 400;
		viewport.dispatchEvent(new Event('resize'));
		await expect.poll(() => dialog.getBoundingClientRect().bottom).toBeLessThanOrEqual(400);
		expect(dialog.open).toBe(true);
		await unmount();
		expect(document.activeElement).toBe(trigger);
		trigger.remove();
	});

	it.each(['long drag', 'fresh tap', 'keyboard'])('ignores clicks after a short handle drag, then closes with %s', async (dismissal) => {
		await page.viewport(390, 844);
		const { container } = render(ResponsiveDialogHarness);
		const dialog = container.querySelector('dialog')!;
		const handle = dialog.querySelector('button')!;
		vi.spyOn(handle, 'setPointerCapture').mockImplementation(() => {});
		vi.spyOn(handle, 'releasePointerCapture').mockImplementation(() => {});
		function drag(target: Element, distance: number) {
			for (const [type, y, time, receiver] of [
				['pointerdown', 0, 0, target],
				['pointermove', distance, 1000, window],
				['pointerup', distance, 1001, window]
			] as const) {
				const event = new PointerEvent(type, {
					bubbles: true,
					pointerId: 1,
					pointerType: 'mouse',
					clientY: y
				});
				Object.defineProperty(event, 'timeStamp', { value: time });
				receiver.dispatchEvent(event);
			}
			// The browser can emit a click after a captured pointer drag.
			target.dispatchEvent(new MouseEvent('click', { bubbles: true, detail: 1 }));
		}
		drag(dialog.querySelector('textarea')!, 500);
		await tick();
		expect(dialog.classList.contains('closing')).toBe(false);
		drag(handle, 12);
		await tick();
		expect(dialog.classList.contains('closing')).toBe(false);
		expect(dialog.style.getPropertyValue('--modal-drag-offset')).toBe('0px');
		if (dismissal === 'long drag') drag(handle, 500);
		else if (dismissal === 'fresh tap') drag(handle, 0);
		else {
			handle.focus();
			await userEvent.keyboard('{Enter}');
		}
		await tick();
		expect(dialog.classList.contains('closing')).toBe(true);
	});

	it('ignores mobile keyboard cancel while editing, but explicit Close still works without animation', async () => {
		await page.viewport(390, 844);
		const onclose = vi.fn();
		const { container } = render(ResponsiveDialogHarness, { onclose });
		const dialog = container.querySelector('dialog')!;
		dialog.querySelector('textarea')!.focus();
		dialog.dispatchEvent(new Event('cancel', { cancelable: true }));
		await tick();
		expect(dialog.classList.contains('closing')).toBe(false);
		dialog.style.animation = 'none';
		dialog.querySelector<HTMLButtonElement>('header button')!.click();
		await expect.poll(() => dialog.open).toBe(false);
		// `close()` clears `open` at once, but the `close` event is a later task.
		await expect.poll(() => onclose.mock.calls.length).toBe(1);
	});
});
