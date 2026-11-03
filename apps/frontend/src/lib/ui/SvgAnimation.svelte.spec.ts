import '../../app.css';
import { afterEach, describe, expect, it } from 'vitest';
import { userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import { sanitizeSvg } from '$lib/sanitizeSvg';
import { applyAnimationPreference } from '$lib/dom/motionPreference';
import SvgAnimation from './SvgAnimation.svelte';

const source = sanitizeSvg(
  '<svg xmlns="http://www.w3.org/2000/svg" width="200" height="100"><circle cx="10" cy="50" r="8"><animate attributeName="cx" from="10" to="190" dur="4s" repeatCount="indefinite"/></circle></svg>'
);

/** Wait for browser animation frames rather than assuming a timer advances SVG time. */
async function nextFrames() {
  for (let i = 0; i < 4; i++) await new Promise(requestAnimationFrame);
}

afterEach(() => applyAnimationPreference('automatic'));

describe('SVG animation playback', () => {
  it.each(['none', 'xMidYMid slice'])(
    'fits the artwork viewport before applying preserveAspectRatio="%s"',
    async (preserveAspectRatio) => {
      const view = render(SvgAnimation, {
        source: source.replace('<svg ', `<svg preserveAspectRatio="${preserveAspectRatio}" `),
        alt: 'Moving circle',
        width: 200,
        height: 100
      });
      view.container.style.cssText = 'width:400px;height:400px';
      try {
        const frame = view.container.querySelector('iframe')!;
        const bounds = () => frame.contentDocument?.querySelector('svg')?.getBoundingClientRect();
        await expect.poll(() => bounds()?.width).toBe(400);
        await expect.poll(() => bounds()?.height).toBe(200);
        view.container.style.cssText = 'width:44px;height:200px';
        await expect.poll(() => bounds()?.width).toBe(44);
        await expect.poll(() => bounds()?.height).toBe(22);
      } finally {
        await view.unmount();
      }
    }
  );

  it('pauses the actual timeline and resumes at the same position', async () => {
    applyAnimationPreference('enabled');
    const view = render(SvgAnimation, { source, alt: 'Moving circle', width: 200, height: 100 });
    view.container.style.cssText = 'width:200px;height:100px';
    try {
      const frame = view.container.querySelector('iframe')!;
      const root = () => frame.contentDocument?.querySelector('svg');
      await expect.poll(() => root()?.getCurrentTime() ?? 0).toBeGreaterThan(0);
      const originalRoot = root();
      const toggle = view.getByRole('button', { name: 'Pause animation' });
      await expect.element(toggle).toHaveAttribute('aria-pressed', 'false');
      await userEvent.click(toggle);
      await expect.element(toggle).toHaveAccessibleName('Pause animation');
      await expect.element(toggle).toHaveAttribute('aria-pressed', 'true');
      const time = root()!.getCurrentTime();
      const position = root()!.querySelector('circle')!.cx.animVal.value;
      await nextFrames();
      expect(root()!.getCurrentTime()).toBe(time);
      expect(root()!.querySelector('circle')!.cx.animVal.value).toBe(position);
      expect(root()!.animationsPaused()).toBe(true);
      expect(frame.getAttribute('sandbox')).toBe('allow-same-origin');
      expect(frame.contentDocument!.querySelector('meta')!.content).toContain("default-src 'none'");
      await userEvent.click(toggle);
      await expect.element(toggle).toHaveAccessibleName('Pause animation');
      await expect.element(toggle).toHaveAttribute('aria-pressed', 'false');
      await expect.poll(() => root()!.getCurrentTime()).toBeGreaterThan(time);
      expect(root()!.animationsPaused()).toBe(false);
      expect(root()).toBe(originalRoot);
    } finally {
      await view.unmount();
    }
  });

  it('pauses hidden artwork and replaces its source without replacing the frame', async () => {
    applyAnimationPreference('enabled');
    const view = render(SvgAnimation, { source, alt: 'Moving circle', width: 200, height: 100 });
    view.container.style.cssText = 'width:200px;height:100px';
    let lastRoot: SVGSVGElement | undefined;
    try {
      const frame = view.container.querySelector('iframe')!;
      const root = () => frame.contentDocument?.querySelector('svg');
      await expect.poll(() => root()?.getCurrentTime() ?? 0).toBeGreaterThan(0);
      view.container.style.display = 'none';
      await expect.poll(() => root()!.animationsPaused()).toBe(true);
      const time = root()!.getCurrentTime();
      await nextFrames();
      expect(root()!.getCurrentTime()).toBe(time);
      view.container.style.display = 'block';
      await expect.poll(() => root()!.getCurrentTime()).toBeGreaterThan(time);
      await view.rerender({ source: source.replace('r="8"', 'r="12"') });
      await expect.poll(() => root()?.querySelector('circle')?.getAttribute('r')).toBe('12');
      expect(view.container.querySelector('iframe')).toBe(frame);
      lastRoot = root()!;
    } finally {
      await view.unmount();
    }
    expect(lastRoot!.animationsPaused()).toBe(true);
  });

  it('blocks scripts and resource loads even when inserted after sanitization', async () => {
    const view = render(SvgAnimation, { source, alt: 'Moving circle', width: 200, height: 100 });
    try {
      const frame = view.container.querySelector('iframe')!;
      await expect.poll(() => frame.contentDocument?.querySelector('svg')).toBeTruthy();
      const document = frame.contentDocument!;
      const blocked: string[] = [];
      document.addEventListener('securitypolicyviolation', (event) =>
        blocked.push(event.effectiveDirective)
      );
      const script = document.createElement('script');
      script.textContent = 'document.body.dataset.scriptRan = "yes"';
      document.body.append(script);
      const image = document.createElement('img');
      image.src = 'http://127.0.0.1:1/svg-policy-probe';
      document.body.append(image);
      await expect.poll(() => blocked).toContain('img-src');
      expect(document.body.dataset.scriptRan).toBeUndefined();
    } finally {
      await view.unmount();
    }
  });

  it('starts paused for reduced motion and permits explicit playback', async () => {
    applyAnimationPreference('disabled');
    const view = render(SvgAnimation, { source, alt: 'Moving circle', width: 200, height: 100 });
    view.container.style.cssText = 'width:200px;height:100px';
    try {
      const frame = view.container.querySelector('iframe')!;
      await expect
        .poll(() => frame.contentDocument?.querySelector('svg')?.animationsPaused())
        .toBe(true);
      const toggle = view.getByRole('button', { name: 'Pause animation' });
      await expect.element(toggle).toHaveAttribute('aria-pressed', 'true');
      await userEvent.click(toggle);
      await expect.element(toggle).toHaveAttribute('aria-pressed', 'false');
      await expect
        .poll(() => frame.contentDocument!.querySelector('svg')!.getCurrentTime())
        .toBeGreaterThan(0);
      applyAnimationPreference('disabled');
      await expect
        .poll(() => frame.contentDocument!.querySelector('svg')!.animationsPaused())
        .toBe(true);
      await expect.element(toggle).toHaveAttribute('aria-pressed', 'true');
    } finally {
      await view.unmount();
    }
  });
});
