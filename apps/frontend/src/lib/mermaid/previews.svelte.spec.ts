import '../../app.css';
import { expect, it, vi } from 'vitest';
import { createMermaidPreview } from './previews';

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
