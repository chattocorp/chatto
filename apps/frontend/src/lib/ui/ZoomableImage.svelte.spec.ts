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
  const stage = view.container.querySelector<HTMLDivElement>('.touch-none')!;
  await expect.poll(() => stage.clientWidth).toBe(400);
  await expect.poll(() => view.container.querySelector('img')?.naturalWidth).toBe(400);
  await expect
    .poll(() => view.container.querySelector('img')?.classList.contains('skeleton'))
    .toBe(false);
  return { query: view, stage, image: view.container.querySelector<HTMLImageElement>('img')! };
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
  it('steps from Fit to 400%, disables the limit, and returns to Fit', async () => {
    const view = await mount();
    const increase = view.query.getByRole('button', { name: 'Zoom in' });
    const decrease = view.query.getByRole('button', { name: 'Zoom out' });
    await expect.element(decrease).toBeDisabled();
    for (let step = 0; step < 12; step += 1) await increase.click();
    await expect.element(view.query.getByText('400%')).toBeVisible();
    await expect.element(increase).toBeDisabled();
    await decrease.click();
    await expect.element(view.query.getByText('375%')).toBeVisible();
    await view.query.getByRole('button', { name: 'Fit' }).click();
    await expect.element(view.query.getByText('100%')).toBeVisible();
    expect(view.image.style.transform).toBe('translate(0px, 0px) scale(1)');
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
      deltaY: -Math.log(2) / 0.002
    });
    view.stage.dispatchEvent(wheel);
    expect(wheel.defaultPrevented).toBe(true);
    await expect.element(view.query.getByText('200%')).toBeVisible();
    expect(view.image.style.transform).toMatch(/translate\(-100(?:\.\d+)?px, 0px\) scale\(2/);

    pointer(view.stage, 'pointerdown', 1, rect.left + 200, rect.top + 150);
    pointer(view.stage, 'pointermove', 1, rect.left + 2000, rect.top + 150);
    await tick();
    const fit = Math.min(view.stage.clientWidth / 400, view.stage.clientHeight / 300);
    const maxX = Math.max(0, (400 * fit * 2 - view.stage.clientWidth) / 2);
    expect(view.image.style.transform).toContain(`translate(${maxX}px, 0px)`);
    pointer(view.stage, 'pointerup', 1, rect.left + 2000, rect.top + 150);
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
