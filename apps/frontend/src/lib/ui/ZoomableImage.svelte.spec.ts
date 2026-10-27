import '../../app.css';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { tick } from 'svelte';
import ZoomableImage from './ZoomableImage.svelte';

const src = `data:image/svg+xml,${encodeURIComponent(
  '<svg xmlns="http://www.w3.org/2000/svg" width="400" height="300"></svg>'
)}`;

async function mount() {
  const view = render(ZoomableImage, { props: { src, alt: 'Test image', onerror: vi.fn() } });
  view.container.style.width = '400px';
  view.container.style.height = '350px';
  expect(view.container.querySelector('img')?.classList.contains('skeleton')).toBe(false);
  const stage = view.container.querySelector<HTMLDivElement>('[role="group"]')!;
  await expect.poll(() => stage.clientWidth).toBe(400);
  await expect.poll(() => view.container.querySelector('img')?.naturalWidth).toBe(400);
  await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
  return {
    query: view,
    stage,
    image: view.container.querySelector<HTMLImageElement>('img')!,
    layer: view.container.querySelector<HTMLElement>('.image-canvas-layer')!
  };
}

function pointer(stage: HTMLElement, type: string, id: number, x: number, y: number) {
  stage.dispatchEvent(
    new PointerEvent(type, {
      bubbles: true,
      pointerId: id,
      pointerType: 'touch',
      clientX: x,
      clientY: y
    })
  );
}

describe('ZoomableImage', () => {
  it('pans a bounded inline image, releases wheel scrolling at edges and shares zoom and Fit', async () => {
    const image = new Image();
    image.src = `data:image/svg+xml,${encodeURIComponent('<svg xmlns="http://www.w3.org/2000/svg" width="400" height="1200"/>')}`;
    image.alt = 'Tall diagram';
    const view = render(ZoomableImage, { image, inline: true, onerror: vi.fn() });
    view.container.style.width = '900px';
    const stage = view.container.querySelector<HTMLElement>('[role="group"]')!;
    const layer = view.container.querySelector<HTMLElement>('.image-canvas-layer')!;
    await expect.poll(() => image.naturalHeight).toBe(1200);
    await expect.poll(() => stage.clientWidth).toBe(900);
    expect(view.container.querySelector('output')?.textContent).toBe('100%');
    expect(layer.style.transform).toBe('translate(0px, 0px) scale(1)');
    expect(stage.clientHeight).toBeLessThanOrEqual(Math.min(480, innerHeight * 0.6));
    expect(view.container.querySelector('img')).toBe(image);
    expect(stage.getBoundingClientRect().width).toBeCloseTo(900, 0);
    const canvas = view.container.querySelector<HTMLElement>('.image-canvas')!;
    expect(canvas.clientHeight).toBe(stage.clientHeight);
    const controls = canvas.querySelector('output')!.parentElement!;
    expect(controls.getBoundingClientRect().bottom).toBeLessThan(
      stage.getBoundingClientRect().bottom
    );
    await view.getByRole('button', { name: 'Zoom in' }).click();
    const initialZoom = view.container.querySelector('output')!.textContent;
    const firstTransform = layer.style.transform;
    const wheel = (deltaY: number, ctrlKey = false) => {
      const event = new WheelEvent('wheel', { bubbles: true, cancelable: true, deltaY, ctrlKey });
      stage.dispatchEvent(event);
      return event;
    };
    expect(wheel(-10000).defaultPrevented).toBe(true);
    expect(wheel(-100).defaultPrevented).toBe(false);
    expect(wheel(100).defaultPrevented).toBe(true);
    await tick();
    expect(layer.style.transform).not.toBe(firstTransform);
    expect(view.container.querySelector('output')!.textContent).toBe(initialZoom);
    expect(wheel(10000).defaultPrevented).toBe(true);
    expect(wheel(100).defaultPrevented).toBe(false);
    expect(wheel(-100, true).defaultPrevented).toBe(true);
    await tick();
    expect(view.container.querySelector('output')!.textContent).not.toBe(initialZoom);
    await view.getByRole('button', { name: 'Fit' }).click();
    expect(layer.style.transform).toBe('translate(0px, 0px) scale(1)');
    expect(wheel(100).defaultPrevented).toBe(false);
    await expect
      .poll(() =>
        [...view.container.querySelectorAll('.image-canvas-fade')].every((fade) =>
          fade.classList.contains('opacity-0')
        )
      )
      .toBe(true);
  });

  it('keeps inline gestures away from message activation and scopes keyboard controls to focus', async () => {
    const view = render(ZoomableImage, {
      src,
      alt: 'Inline image',
      inline: true,
      onerror: vi.fn()
    });
    view.container.style.width = '400px';
    const parentGesture = vi.fn();
    for (const type of ['click', 'mousedown', 'mouseup', 'touchstart', 'touchend'])
      view.container.addEventListener(type, parentGesture);
    const outsidePress = vi.fn();
    document.addEventListener('pointerdown', outsidePress);
    await expect.poll(() => view.container.querySelector('img')?.naturalWidth).toBe(400);
    const increase = view.getByRole('button', { name: 'Zoom in' });
    await increase.click();
    await expect.element(view.getByText('125%')).toBeVisible();
    expect(parentGesture).not.toHaveBeenCalled();
    expect(outsidePress).toHaveBeenCalled();
    document.removeEventListener('pointerdown', outsidePress);
    window.dispatchEvent(
      new KeyboardEvent('keydown', { key: '+', bubbles: true, cancelable: true })
    );
    await expect.element(view.getByText('150%')).toBeVisible();
    (document.activeElement as HTMLElement).blur();
    window.dispatchEvent(
      new KeyboardEvent('keydown', { key: '+', bubbles: true, cancelable: true })
    );
    await expect.element(view.getByText('150%')).toBeVisible();
  });

  it('steps beyond 400% to 1600%, disables the limit, and returns to Fit', async () => {
    const view = await mount();
    const increase = view.query.getByRole('button', { name: 'Zoom in' });
    const decrease = view.query.getByRole('button', { name: 'Zoom out' });
    await expect.element(decrease).toBeDisabled();
    for (let step = 0; step < 12; step += 1) await increase.click();
    await expect.element(view.query.getByText('400%')).toBeVisible();
    await expect.element(increase).toBeEnabled();
    for (let step = 0; step < 48; step += 1) await increase.click();
    await expect.element(view.query.getByText('1600%')).toBeVisible();
    await expect.element(increase).toBeDisabled();
    await decrease.click();
    await expect.element(view.query.getByText('1575%')).toBeVisible();
    await view.query.getByRole('button', { name: 'Fit' }).click();
    await expect.element(view.query.getByText('100%')).toBeVisible();
    expect(view.layer.style.transform).toBe('translate(0px, 0px) scale(1)');
  });

  it('keeps the wheel focal point steady and clamps dragging to the image', async () => {
    const view = await mount();
    view.stage.setPointerCapture = vi.fn();
    view.stage.hasPointerCapture = () => false;
    const rect = view.stage.getBoundingClientRect();
    const wheel = new WheelEvent('wheel', {
      bubbles: true,
      cancelable: true,
      clientX: rect.left + 300,
      clientY: rect.top + rect.height / 2,
      deltaY: -Math.log(8) / 0.002
    });
    view.stage.dispatchEvent(wheel);
    expect(wheel.defaultPrevented).toBe(true);
    await expect.element(view.query.getByText('800%')).toBeVisible();
    expect(view.layer.style.transform).toMatch(/translate\(-700(?:\.\d+)?px, 0px\) scale\(8/);

    // Events that start on the image bubble to the stage's pan handlers.
    pointer(view.image, 'pointerdown', 1, rect.left + 200, rect.top + 150);
    pointer(view.image, 'pointermove', 1, rect.left + 10000, rect.top + 150);
    await tick();
    const fit = Math.min(view.stage.clientWidth / 400, view.stage.clientHeight / 300);
    const maxX = Math.max(0, (400 * fit * 8 - view.stage.clientWidth) / 2);
    expect(view.layer.style.transform).toContain(`translate(${maxX}px, 0px)`);
    pointer(view.image, 'pointerup', 1, rect.left + 2000, rect.top + 150);
    view.stage.dispatchEvent(new WheelEvent('wheel', { deltaY: -10000, cancelable: true }));
    await expect.element(view.query.getByText('1600%')).toBeVisible();
    await expect.element(view.query.getByRole('button', { name: 'Zoom in' })).toBeDisabled();
    view.stage.dispatchEvent(new WheelEvent('wheel', { deltaY: 10000, cancelable: true }));
    await expect.element(view.query.getByText('100%')).toBeVisible();
    expect(view.layer.style.transform).toBe('translate(0px, 0px) scale(1)');
  });

  it('keeps the image as the hit target so the native image context menu works', async () => {
    const view = await mount();
    const rect = view.stage.getBoundingClientRect();
    expect(document.elementFromPoint(rect.left + rect.width / 2, rect.top + rect.height / 2)).toBe(
      view.image
    );
    const menu = new MouseEvent('contextmenu', { bubbles: true, cancelable: true, button: 2 });
    view.image.dispatchEvent(menu);
    expect(menu.defaultPrevented).toBe(false);
  });

  it('forgets a touch whose pointer capture ends without pointerup', async () => {
    const view = await mount();
    view.stage.setPointerCapture = vi.fn();
    view.stage.hasPointerCapture = () => false;
    const rect = view.stage.getBoundingClientRect();
    // A long-press image menu can end a touch without pointerup.
    pointer(view.image, 'pointerdown', 1, rect.left + 150, rect.top + 150);
    pointer(view.stage, 'lostpointercapture', 1, rect.left + 150, rect.top + 150);
    pointer(view.image, 'pointerdown', 2, rect.left + 200, rect.top + 150);
    pointer(view.image, 'pointermove', 2, rect.left + 300, rect.top + 150);
    await tick();
    await expect.element(view.query.getByText('100%')).toBeVisible();
    pointer(view.image, 'pointerup', 2, rect.left + 300, rect.top + 150);
  });

  it('zooms with a two-pointer pinch and toggles with double-click', async () => {
    const view = await mount();
    view.stage.setPointerCapture = vi.fn();
    view.stage.hasPointerCapture = () => false;
    const rect = view.stage.getBoundingClientRect();
    pointer(view.stage, 'pointerdown', 1, rect.left + 150, rect.top + 150);
    pointer(view.stage, 'pointerdown', 2, rect.left + 250, rect.top + 150);
    pointer(view.stage, 'pointermove', 2, rect.left + 350, rect.top + 150);
    await expect.element(view.query.getByText('200%')).toBeVisible();
    pointer(view.stage, 'pointerup', 1, rect.left + 150, rect.top + 150);
    pointer(view.stage, 'pointerup', 2, rect.left + 350, rect.top + 150);
    view.stage.dispatchEvent(
      new MouseEvent('dblclick', {
        bubbles: true,
        clientX: rect.left + 200,
        clientY: rect.top + 150
      })
    );
    await expect.element(view.query.getByText('100%')).toBeVisible();
  });
});
