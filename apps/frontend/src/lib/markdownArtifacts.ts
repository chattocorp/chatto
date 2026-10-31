/** Local tab state and lazy image jobs for one mounted Markdown HTML version. */
import { createMermaidPreview } from './mermaid/previews';
import { mount, unmount } from 'svelte';

/** Scope ARIA IDs to the component, and dispose all jobs when its HTML changes. */
export function attachCodeFrames(html: string, scope: string) {
  return (element: HTMLDivElement) => {
    if (!html.includes('data-artifact')) return;
    const controller = new AbortController();
    const { signal } = controller;
    const canvases: (() => void)[] = [];
    const starts = new Map<Element, () => void>();
    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (!entry.isIntersecting) continue;
          observer.unobserve(entry.target);
          starts.get(entry.target)?.();
        }
      },
      { rootMargin: '200px' }
    );

    element.querySelectorAll<HTMLElement>('[data-artifact]').forEach((frame, index) => {
      const tabs = [...frame.querySelectorAll<HTMLButtonElement>('[data-code-tab]')];
      const panels = [...frame.querySelectorAll<HTMLElement>('[data-code-panel]')];
      const [sourcePanel, renderPanel] = panels;
      const error = frame.querySelector<HTMLElement>('.markdown-code-error')!;
      const source =
        sourcePanel.querySelector<HTMLElement>('[data-copy-source]')!.dataset.copySource!;
      let selected = tabs[1].getAttribute('aria-selected') === 'true' ? 1 : 0;
      let nearby = false;
      let started = false;

      function select(next: number) {
        if (tabs[next].disabled) return;
        selected = next;
        tabs.forEach((tab, i) => {
          tab.setAttribute('aria-selected', String(i === next));
          tab.tabIndex = i === next ? 0 : -1;
          panels[i].hidden = i !== next;
        });
        void start();
      }
      function fail() {
        if (signal.aborted) return;
        const focused = document.activeElement === tabs[1];
        select(0);
        tabs[1].disabled = true;
        error.hidden = false;
        if (focused) tabs[0].focus();
      }
      async function start() {
        if (!nearby || selected !== 1 || started || tabs[1].disabled) return;
        started = true;
        if (frame.dataset.artifact === 'ansi') return;
        renderPanel.setAttribute('aria-busy', 'true');
        try {
          const image =
            frame.dataset.artifact === 'mermaid'
              ? await createMermaidPreview(source, signal)
              : new Image();
          if (signal.aborted) return;
          if (!image) {
            fail();
            return;
          }
          image.addEventListener('error', fail, { signal });
          image.addEventListener('load', () => renderPanel.removeAttribute('aria-busy'), {
            signal
          });
          if (frame.dataset.artifact === 'svg') {
            image.className = 'markdown-svg-preview';
            image.alt = 'SVG';
            image.decoding = 'async';
            image.src = `data:image/svg+xml;charset=utf-8,${encodeURIComponent(source)}`;
          }
          const { RenderedImage } = await import('$lib/ui/attachments');
          if (signal.aborted) return;
          const canvas = mount(RenderedImage, {
            target: renderPanel,
            props: { image }
          });
          canvases.push(() => {
            void unmount(canvas);
          });
          if (image.complete) {
            renderPanel.removeAttribute('aria-busy');
            if (!image.naturalWidth) fail();
          }
        } catch {
          fail();
        }
      }

      tabs.forEach((tab, i) => {
        const id = `${scope}-code-${index}-${i}`;
        tab.id = `${id}-tab`;
        tab.setAttribute('aria-controls', id);
        panels[i].id = id;
        // Visuals have no visible tabs. Their image button (or ANSI code) owns focus.
        if (frame.dataset.presentation !== 'visual' || i === 0) {
          panels[i].role = 'tabpanel';
          panels[i].setAttribute('aria-labelledby', tab.id);
          panels[i].tabIndex = 0;
        }
        tab.addEventListener(
          'click',
          (event) => {
            event.preventDefault();
            event.stopPropagation();
            select(i);
          },
          { signal }
        );
        tab.addEventListener(
          'keydown',
          (event) => {
            const next =
              event.key === 'Home'
                ? 0
                : event.key === 'End'
                  ? 1
                  : ['ArrowLeft', 'ArrowRight'].includes(event.key)
                    ? 1 - i
                    : undefined;
            if (next === undefined || tabs[next].disabled) return;
            event.preventDefault();
            event.stopPropagation();
            select(next);
            tabs[next].focus();
          },
          { signal }
        );
      });
      starts.set(frame, () => {
        nearby = true;
        void start();
      });
      observer.observe(frame);
    });
    return () => {
      controller.abort();
      canvases.forEach((dispose) => dispose());
      observer.disconnect();
      starts.clear();
    };
  };
}
