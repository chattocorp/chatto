import { afterEach, describe, expect, it, vi } from 'vitest';
import { readScrollEdges, trackScrollEdges } from './scrollEdges';
import { attachCodeFades } from './codeScrollFades';

const cleanups: Array<() => void> = [];
afterEach(() => {
  for (const cleanup of cleanups.splice(0)) cleanup();
});

function viewport(dir: 'ltr' | 'rtl' = 'ltr') {
  const element = document.createElement('div');
  element.dir = dir;
  element.style.cssText = 'width: 100px; height: 100px; overflow: auto; position: fixed;';
  const content = document.createElement('div');
  content.style.cssText = 'display: flow-root; width: max-content; min-width: 100%;';
  const child = document.createElement('div');
  child.style.cssText = 'width: 300px; height: 300px;';
  content.append(child);
  element.append(content);
  document.body.append(element);
  cleanups.push(() => element.remove());
  return { element, content, child };
}

describe('scroll edge attachment', () => {
  it.each(['x', 'y'] as const)(
    'can wait for resize delivery before reading the %s axis',
    async (axis) => {
      const { element, content } = viewport();
      const measured = vi.spyOn(element, axis === 'x' ? 'scrollWidth' : 'scrollHeight', 'get');
      const changed = vi.fn();
      try {
        const cleanup = trackScrollEdges(axis, changed, { initialMeasurement: 'resize' })(content);
        if (cleanup) cleanups.push(cleanup);
        expect(measured).not.toHaveBeenCalled();
        expect(changed).not.toHaveBeenCalled();
        await vi.waitFor(() => expect(changed).toHaveBeenCalledWith({ start: false, end: true }));
        expect(measured).toHaveBeenCalled();
      } finally {
        measured.mockRestore();
      }
    }
  );

  it('disposes deferred measurements before the first resize delivery', async () => {
    const { element, content } = viewport();
    const changed = vi.fn();
    const cleanup = trackScrollEdges('y', changed, { initialMeasurement: 'resize' })(content);
    cleanup?.();
    element.dispatchEvent(new Event('scroll'));
    await new Promise<void>((resolve) =>
      requestAnimationFrame(() => requestAnimationFrame(() => resolve()))
    );
    expect(changed).not.toHaveBeenCalled();
  });

  it('defers code fade reads until layout and keeps the viewport size unchanged', async () => {
    const { element, content } = viewport();
    const size = element.getBoundingClientRect();
    const widthRead = vi.spyOn(element, 'scrollWidth', 'get');
    const heightRead = vi.spyOn(element, 'scrollHeight', 'get');
    const frame = document.createElement('div');
    document.body.append(frame);
    cleanups.push(() => frame.remove());
    try {
      cleanups.push(attachCodeFades(content, frame));
      expect(widthRead).not.toHaveBeenCalled();
      expect(heightRead).not.toHaveBeenCalled();
      const visible = (edge: string) =>
        !frame.querySelector(`.markdown-code-fade-${edge}`)!.classList.contains('opacity-0');
      await vi.waitFor(() => expect(visible('bottom') && visible('right')).toBe(true));
      expect(visible('top') || visible('left')).toBe(false);
      const after = element.getBoundingClientRect();
      expect([after.width, after.height]).toEqual([size.width, size.height]);
    } finally {
      widthRead.mockRestore();
      heightRead.mockRestore();
    }
  });

  it.each(['x', 'y'] as const)('tracks both edges on the %s axis', async (axis) => {
    const { element, content } = viewport();
    const changed = vi.fn();
    const cleanup = trackScrollEdges(axis, changed)(content);
    if (cleanup) cleanups.push(cleanup);
    expect(changed).toHaveBeenLastCalledWith({ start: false, end: true });

    element.scrollTo(50, 50);
    await vi.waitFor(() => expect(changed).toHaveBeenLastCalledWith({ start: true, end: true }));
    element.scrollTo(1000, 1000);
    await vi.waitFor(() => expect(changed).toHaveBeenLastCalledWith({ start: true, end: false }));
  });

  it('reports logical inline edges in a right-to-left viewport', async () => {
    const { element, content } = viewport('rtl');
    const changed = vi.fn();
    const cleanup = trackScrollEdges('x', changed)(content);
    if (cleanup) cleanups.push(cleanup);
    expect(changed).toHaveBeenLastCalledWith({ start: false, end: true });

    // Right-to-left scrollLeft starts at 0 and decreases towards the inline end.
    element.scrollTo(-50, 0);
    await vi.waitFor(() => expect(changed).toHaveBeenLastCalledWith({ start: true, end: true }));
    element.scrollTo(-1000, 0);
    await vi.waitFor(() => expect(changed).toHaveBeenLastCalledWith({ start: true, end: false }));
  });

  it('tracks replacement children and their later size changes', async () => {
    const { content } = viewport();
    const changed = vi.fn();
    const cleanup = trackScrollEdges('y', changed)(content);
    if (cleanup) cleanups.push(cleanup);
    const replacement = document.createElement('div');
    replacement.style.height = '50px';
    content.replaceChildren(replacement);
    await vi.waitFor(() => expect(changed).toHaveBeenLastCalledWith({ start: false, end: false }));
    replacement.style.height = '300px';
    await vi.waitFor(() => expect(changed).toHaveBeenLastCalledWith({ start: false, end: true }));
  });

  it('stops reporting scroll and layout changes after detach', async () => {
    const { element, content, child } = viewport();
    const changed = vi.fn();
    const cleanup = trackScrollEdges('y', changed)(content);
    if (cleanup) cleanup();
    changed.mockClear();
    element.dispatchEvent(new Event('scroll'));
    child.style.height = '50px';
    content.replaceChildren(document.createElement('div'));
    // Let native mutation and resize deliveries run before checking cleanup.
    await new Promise<void>((resolve) =>
      requestAnimationFrame(() => requestAnimationFrame(() => resolve()))
    );
    expect(changed).not.toHaveBeenCalled();
  });

  it('handles overscroll and ignores one-pixel rounding differences', () => {
    const { element } = viewport();
    Object.defineProperties(element, {
      scrollTop: { value: -20, configurable: true },
      scrollHeight: { value: 300, configurable: true },
      clientHeight: { value: 100 }
    });
    expect(readScrollEdges(element, 'y')).toEqual({ start: false, end: true });
    Object.defineProperty(element, 'scrollTop', { value: 220 });
    expect(readScrollEdges(element, 'y')).toEqual({ start: true, end: false });
    Object.defineProperty(element, 'scrollHeight', { value: 101 });
    expect(readScrollEdges(element, 'y')).toEqual({ start: false, end: false });
  });

  it('handles right-to-left overscroll past the inline start', () => {
    const { element } = viewport('rtl');
    Object.defineProperties(element, {
      scrollLeft: { value: 20, configurable: true },
      scrollWidth: { value: 300, configurable: true },
      clientWidth: { value: 100 }
    });
    expect(readScrollEdges(element, 'x')).toEqual({ start: false, end: true });
    Object.defineProperty(element, 'scrollLeft', { value: -220 });
    expect(readScrollEdges(element, 'x')).toEqual({ start: true, end: false });
  });
});
