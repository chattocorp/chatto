import { afterEach, describe, expect, it, vi } from 'vitest';
import { readScrollEdges, trackScrollEdges } from './scrollEdges';

const cleanups: Array<() => void> = [];
afterEach(() => {
  for (const cleanup of cleanups.splice(0)) cleanup();
});

function viewport() {
  const element = document.createElement('div');
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
});
