import '../../app.css';
import { expect, it } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import LoadingDots from './LoadingDots.svelte';

it('announces a busy status with its label', async () => {
  render(LoadingDots, { props: { label: 'Loading messages...' } });
  const status = page.getByRole('status', { name: 'Loading messages...' });
  await expect.element(status).toHaveAttribute('aria-busy', 'true');
  expect(status.element().querySelectorAll('.loading-dot')).toHaveLength(3);
});

it('keeps the reveal delay when the user requests less motion', () => {
  const reducedMotionRules = [...document.styleSheets]
    .flatMap((sheet) => [...sheet.cssRules])
    .filter(
      (rule): rule is CSSMediaRule =>
        rule instanceof CSSMediaRule && rule.conditionText.includes('prefers-reduced-motion')
    )
    .flatMap((rule) => [...rule.cssRules])
    .filter(
      (rule): rule is CSSStyleRule =>
        rule instanceof CSSStyleRule && /\.loading-dots\b/.test(rule.selectorText)
    );

  expect(reducedMotionRules.length).toBeGreaterThan(0);
  for (const rule of reducedMotionRules) {
    // Removing the whole animation would also remove the delay and flash the dots.
    expect(rule.style.animationName).not.toBe('none');
  }
});
