import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';

import Deadline from './Deadline.svelte';
import Interval from './Interval.svelte';

describe('lifecycle timers', () => {
	beforeEach(() => {
		vi.useFakeTimers();
	});

	afterEach(() => {
		vi.useRealTimers();
	});

	it('ticks at a fixed interval only while mounted', async () => {
		const ontick = vi.fn();
		const rendered = render(Interval, { props: { milliseconds: 100, ontick } });

		await vi.advanceTimersByTimeAsync(250);
		expect(ontick).toHaveBeenCalledTimes(2);

		rendered.unmount();
		await vi.advanceTimersByTimeAsync(100);
		expect(ontick).toHaveBeenCalledTimes(2);
	});

	it('runs once at an absolute deadline', async () => {
		const onreached = vi.fn();
		const rendered = render(Deadline, {
			props: { at: Date.now() + 100, onreached }
		});

		await vi.advanceTimersByTimeAsync(99);
		expect(onreached).not.toHaveBeenCalled();
		await vi.advanceTimersByTimeAsync(1);
		expect(onreached).toHaveBeenCalledOnce();

		rendered.unmount();
		await vi.runAllTimersAsync();
		expect(onreached).toHaveBeenCalledOnce();
	});

	it('cancels a pending deadline on unmount', async () => {
		const onreached = vi.fn();
		const rendered = render(Deadline, {
			props: { at: Date.now() + 100, onreached }
		});

		rendered.unmount();
		await vi.advanceTimersByTimeAsync(100);
		expect(onreached).not.toHaveBeenCalled();
	});

	it('defers an already-reached deadline until after mount', async () => {
		const onreached = vi.fn();
		render(Deadline, { props: { at: Date.now(), onreached } });

		expect(onreached).not.toHaveBeenCalled();
		await vi.advanceTimersByTimeAsync(0);
		expect(onreached).toHaveBeenCalledOnce();
	});

	it('schedules deadlines beyond the browser timeout limit in chunks', async () => {
		const maximumTimeoutDelayMs = 2_147_483_647;
		const onreached = vi.fn();
		render(Deadline, {
			props: { at: Date.now() + maximumTimeoutDelayMs + 100, onreached }
		});

		await vi.advanceTimersByTimeAsync(maximumTimeoutDelayMs);
		expect(onreached).not.toHaveBeenCalled();
		await vi.advanceTimersByTimeAsync(100);
		expect(onreached).toHaveBeenCalledOnce();
	});
});
