import '../../app.css';
import { cdp, page, userEvent } from 'vitest/browser';
import { afterEach, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { testSnippet } from '$lib/test-utils';
import Frame from './Frame.svelte';

afterEach(async () => {
	await cdp().send('Emulation.setTouchEmulationEnabled', { enabled: false });
	await page.viewport(1280, 720);
});

it.each([false, true])('keeps menu targets and hover controls accessible with touch=%s', async touch => {
	await cdp().send('Emulation.setTouchEmulationEnabled', { enabled: touch });
	const { container } = render(Frame, {
		children: testSnippet('<div class="group/preview relative h-40 w-64"><button class="menu-entry">Command</button><button class="embed-control-button" aria-label="Open attachment">Open</button></div>')
	});
	const command = container.querySelector<HTMLElement>('.menu-entry')!;
	const action = container.querySelector<HTMLElement>('.embed-control-button')!;
	for (const width of [320, 767, 768, 1280]) {
		await page.viewport(width, 800);
		expect(command.getBoundingClientRect().height).toBeGreaterThanOrEqual(touch ? 44 : 32);
		await userEvent.hover(command);
		await expect.poll(() => getComputedStyle(action).opacity).toBe('1');
		await userEvent.unhover(command);
	}
});

it.each([false, true])('keeps frame and timeline presentation in sync with touch=%s', async (touch) => {
	await cdp().send('Emulation.setTouchEmulationEnabled', { enabled: touch });
	const { container } = render(Frame, {
		children: testSnippet('<div class="message-row">Message content</div>')
	});
	const frame = container.firstElementChild!;
	const row = frame.firstElementChild!;
	for (const width of [375, 500, 767, 768, 1024, 375]) {
		await page.viewport(width, 800);
		const mobile = touch && width < 768;
		expect(getComputedStyle(frame).borderRadius).toBe(mobile ? '0px' : '16px');
		expect(getComputedStyle(frame).overflow).toBe(mobile ? 'visible' : 'hidden');
		expect(getComputedStyle(row).marginLeft).toBe(mobile ? '0px' : '8px');
		expect(getComputedStyle(document.documentElement).getPropertyValue('--app-header-height').trim())
			.toBe(mobile ? '3.5rem' : '2.75rem');
	}
});

it.each([false, true])('keeps embed controls reachable with touch=%s at any width', async (touch) => {
	await cdp().send('Emulation.setTouchEmulationEnabled', { enabled: touch });
	expect(matchMedia('(any-hover: hover) and (any-pointer: fine)').matches).toBe(!touch);
	const { container } = render(Frame, {
		children: testSnippet('<div><div class="group/preview relative h-40 w-64"><button class="embed-control-button" aria-label="Open attachment">Open</button></div><button data-testid="outside-preview">Outside preview</button></div>')
	});
	const button = container.querySelector<HTMLButtonElement>('.embed-control-button')!;
	const outside = container.querySelector<HTMLElement>('[data-testid="outside-preview"]')!;
	// Start focused so the test also proves it clears keyboard reveal state.
	button.focus();
	await expect.poll(() => getComputedStyle(button).opacity).toBe('1');
	for (const width of [375, 1024]) {
		await page.viewport(width, 800);
		// Both pointer and keyboard focus can reveal the preview controls.
		await userEvent.hover(outside);
		outside.focus();
		expect(button.parentElement!.matches(':hover')).toBe(false);
		expect(button.parentElement!.matches(':focus-within')).toBe(false);
		await expect.poll(() => getComputedStyle(button).opacity).toBe(touch ? '1' : '0');
		button.focus();
		await expect.poll(() => getComputedStyle(button).opacity).toBe('1');
		outside.focus();
		await expect.poll(() => getComputedStyle(button).opacity).toBe(touch ? '1' : '0');
	}
});
