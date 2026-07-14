import '../../app.css';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import { afterEach, describe, expect, it } from 'vitest';
import DesignSystemVisualHarness from './DesignSystemVisualHarness.svelte';

const cases = [
  { name: 'light desktop', theme: 'light', layout: 'desktop' },
  { name: 'dark desktop', theme: 'dark', layout: 'desktop' },
  { name: 'light mobile', theme: 'light', layout: 'mobile' },
  { name: 'dark mobile', theme: 'dark', layout: 'mobile' }
] as const;

afterEach(() => {
  document.documentElement.dataset.theme = 'light';
  document.getElementById('visual-regression-stability')?.remove();
});

describe('design-system visual regression', () => {
  for (const visualCase of cases) {
    it(visualCase.name, async () => {
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
      render(DesignSystemVisualHarness, { props: { layout: visualCase.layout } });
      await document.fonts.ready;
      await new Promise<void>((resolve) =>
        requestAnimationFrame(() => requestAnimationFrame(() => resolve()))
      );

      await expect
        .element(page.getByTestId('design-system-visual-harness'))
        .toMatchScreenshot(visualCase.name.replace(' ', '-'), {
          comparatorName: 'pixelmatch',
          comparatorOptions: {
            allowedMismatchedPixelRatio: 0.01,
            threshold: 0.15
          }
        });
    });
  }
});
