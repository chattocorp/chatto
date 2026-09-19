import '../../app.css';
import { expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { tick } from 'svelte';
import { testSnippet } from '$lib/test-utils';
import FloatingPopover from './FloatingPopover.svelte';

it('keeps the full menu inside the viewport while its entrance is scaled', async () => {
	const { container } = render(FloatingPopover, {
		position: { x: window.innerWidth - 8, y: window.innerHeight - 8 },
		class: 'menu overlay-enter w-64',
		onclose: vi.fn(),
		children: testSnippet('<div class="h-40">Menu content</div>')
	});
	await tick();
	const popover = container.querySelector<HTMLElement>('[popover]')!;
	const animation = popover.getAnimations()[0];
	expect(animation).toBeDefined();
	animation.pause();
	animation.currentTime = 0;
	// Let the size observer run while the surface is at its initial scale.
	await new Promise(requestAnimationFrame);
	expect(parseFloat(popover.style.left) + popover.offsetWidth).toBeLessThanOrEqual(window.innerWidth - 8);
	expect(parseFloat(popover.style.top) + popover.offsetHeight).toBeLessThanOrEqual(window.innerHeight - 8);
});
