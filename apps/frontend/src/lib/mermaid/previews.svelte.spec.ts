import '../../app.css';
import { expect, it, vi } from 'vitest';
import { createMermaidPreview } from './previews';

it('applies the current theme to a reused layout and honours cancellation', async () => {
  const owner = {};
  const source = 'flowchart LR\nA[Cached]-->B[Theme]';
  const root = document.documentElement;
  const previous = root.style.getPropertyValue('--color-action');
  const transition = root.style.transition;
  const first = new AbortController();
  const second = new AbortController();
  const jobs = vi.spyOn(Worker.prototype, 'postMessage');
  try {
    root.style.transition = 'none';
    root.style.setProperty('--color-action', '#112233');
    const original = await createMermaidPreview(source, first.signal, owner);
    await original!.decode();
    const previousUrl = original!.src;
    first.abort();
    jobs.mockClear();
    root.style.setProperty('--color-action', '#445566');
    const remounted = await createMermaidPreview(source, second.signal, owner, original!);
    expect(remounted).toBe(original);
    expect(remounted!.src).not.toBe(previousUrl);
    expect(decodeURIComponent(remounted!.src)).toContain('--pele-accent:rgb(68, 85, 102);');
    expect(jobs).not.toHaveBeenCalled();
    second.abort();
    expect(await createMermaidPreview(source, second.signal, owner)).toBeNull();
  } finally {
    first.abort();
    second.abort();
    jobs.mockRestore();
    if (previous) root.style.setProperty('--color-action', previous);
    else root.style.removeProperty('--color-action');
    root.style.transition = transition;
  }
});

it('updates only for root theme changes and stops observing when disposed', async () => {
  const controller = new AbortController();
  const root = document.documentElement;
  const button = document.createElement('button');
  document.body.append(button);
  const computed = vi.spyOn(window, 'getComputedStyle');
  const rootReads = () => computed.mock.calls.filter(([element]) => element === root).length;
  const attributes = ['data-theme', 'data-accent', 'data-light-tone', 'data-dark-tone', 'style'];
  const previous = attributes.map((name) => root.getAttribute(name));
  try {
    const image = await createMermaidPreview('flowchart LR\nA-->B', controller.signal);
    expect(image).not.toBeNull();
    computed.mockClear();
    for (let i = 0; i < 10; i++) {
      button.dispatchEvent(new TransitionEvent('transitionend', { bubbles: true }));
    }
    expect(rootReads()).toBe(0);

    root.dispatchEvent(new TransitionEvent('transitionend'));
    expect(rootReads()).toBe(1);
    computed.mockClear();
    root.setAttribute('data-mermaid-test', 'unrelated');
    await new Promise<void>((resolve) => queueMicrotask(resolve));
    expect(rootReads()).toBe(0);

    for (const attribute of attributes) {
      computed.mockClear();
      // Even a same-value write must reach the observer for each supported attribute.
      root.setAttribute(attribute, root.getAttribute(attribute) ?? '');
      await new Promise<void>((resolve) => queueMicrotask(resolve));
      expect(rootReads()).toBe(1);
    }

    controller.abort();
    computed.mockClear();
    root.dispatchEvent(new TransitionEvent('transitionend'));
    root.setAttribute('data-theme', root.getAttribute('data-theme') ?? '');
    await new Promise<void>((resolve) => queueMicrotask(resolve));
    expect(rootReads()).toBe(0);
  } finally {
    controller.abort();
    computed.mockRestore();
    button.remove();
    root.removeAttribute('data-mermaid-test');
    attributes.forEach((attribute, i) => {
      if (previous[i] === null) root.removeAttribute(attribute);
      else root.setAttribute(attribute, previous[i]);
    });
  }
});
