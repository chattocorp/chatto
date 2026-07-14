import '../../app.css';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import { afterEach, describe, expect, it } from 'vitest';
import DesignSystemVisualHarness from './DesignSystemVisualHarness.svelte';

const cases = [
  { name: 'light desktop', theme: 'light', viewport: { width: 800, height: 900 } },
  { name: 'dark desktop', theme: 'dark', viewport: { width: 800, height: 900 } },
  { name: 'light mobile', theme: 'light', viewport: { width: 375, height: 900 } },
  { name: 'dark mobile', theme: 'dark', viewport: { width: 375, height: 900 } }
] as const;

afterEach(async () => {
  document.documentElement.dataset.theme = 'light';
  document.getElementById('visual-regression-stability')?.remove();
  await page.viewport(414, 896);
});

describe.sequential('design-system visual regression', () => {
  for (const visualCase of cases) {
    it(visualCase.name, async () => {
      await page.viewport(visualCase.viewport.width, visualCase.viewport.height);
      document.documentElement.dataset.theme = visualCase.theme;
      const stabilityStyles = document.createElement('style');
      stabilityStyles.id = 'visual-regression-stability';
      stabilityStyles.textContent = `
        [data-testid='design-system-visual-harness'],
        [data-testid='design-system-visual-harness'] * {
          animation: none !important;
          caret-color: transparent !important;
          transition: none !important;
        }
      `;
      document.head.append(stabilityStyles);
      render(DesignSystemVisualHarness);
      await document.fonts.ready;
      await new Promise<void>((resolve) =>
        requestAnimationFrame(() => requestAnimationFrame(() => resolve()))
      );

      await expect
        .element(page.getByTestId('design-system-visual-harness'))
        .toMatchScreenshot(visualCase.name.replace(' ', '-'), {
          // Full-suite browser contention can delay two identical captures even
          // after fonts and animation have settled. Keep strict stability, but
          // give Chromium enough time to prove it under load.
          timeout: 15_000,
          comparatorName: 'pixelmatch',
          comparatorOptions: {
            allowedMismatchedPixelRatio: 0.01,
            threshold: 0.15
          }
        });
    });
  }
});
