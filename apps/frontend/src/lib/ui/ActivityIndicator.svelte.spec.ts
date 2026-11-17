import '../../app.css';
import { afterEach, describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import ActivityIndicator from './ActivityIndicator.svelte';

afterEach(() => {
  delete document.documentElement.dataset.motion;
});

function animationName(container: HTMLElement): string {
  const column = container.querySelector('[data-activity-indicator] > span');
  if (!column) throw new Error('missing activity indicator frames');
  return getComputedStyle(column).animationName;
}

describe('ActivityIndicator', () => {
  // Chatto's motion setting decides, not only the operating system preference.
  it('animates with full motion and stops with reduced motion', () => {
    document.documentElement.dataset.motion = 'full';
    const animated = render(ActivityIndicator);
    expect(animationName(animated.container)).toBe('braille-spinner');
    document.documentElement.dataset.motion = 'reduced';
    expect(animationName(animated.container)).toBe('none');
  });
});
