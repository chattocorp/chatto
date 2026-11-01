/** Local tab state and lazy image jobs for one mounted Markdown HTML version. */
import { createMermaidPreview } from './mermaid/previews';
import { mount, unmount } from 'svelte';
import {
  getRenderedImageSize,
  rememberRenderedImageSize,
  renderedImageSizeKey,
  renderedImageWidth
} from './renderedImageSize';

/** Scope ARIA IDs to the component, and dispose all jobs when its HTML changes. */
export function attachCodeFrames(
  html: string,
  scope: string,
  showSource: (frame: HTMLElement, source: string, language: string) => (() => void) | undefined
) {
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
      let visual = frame.dataset.presentation === 'visual';
      let tabs = [...frame.querySelectorAll<HTMLButtonElement>('[data-code-tab]')];
      let panels = [...frame.querySelectorAll<HTMLElement>('[data-code-panel]')];
      const renderPanel = frame.querySelector<HTMLElement>('[data-code-panel="render"]')!;
      const source =
        frame.dataset.artifactSource ??
        frame.querySelector<HTMLElement>('[data-copy-source]')!.dataset.copySource!;
      const sizeKey = renderedImageSizeKey(frame.dataset.artifact!, source);
      const size = getRenderedImageSize(sizeKey);
      // Reserve geometry before IntersectionObserver or the worker can run. A
      // recycled row must not replace virtua's measured height with an empty panel.
      const placeholder = size ? document.createElement('div') : undefined;
      if (placeholder && size) {
        placeholder.className = 'rendered-image rendered-image-size';
        placeholder.style.width = renderedImageWidth(size);
        placeholder.style.aspectRatio = `${size.width} / ${size.height}`;
        placeholder.setAttribute('aria-hidden', 'true');
        renderPanel.append(placeholder);
        canvases.push(() => placeholder.remove());
      }
      let selected = visual ? 1 : 0;
      let nearby = false;
      let started = false;
      let failed = false;
      let disposeCanvas: (() => void) | undefined;

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
        if (signal.aborted || failed) return;
        failed = true;
        placeholder?.remove();
        disposeCanvas?.();
        if (visual) {
          const cleanup = showSource(frame, source, frame.dataset.artifact!);
          if (cleanup) canvases.push(cleanup);
          visual = false;
          tabs = [...frame.querySelectorAll<HTMLButtonElement>('[data-code-tab]')];
          panels = [...frame.querySelectorAll<HTMLElement>('[data-code-panel]')];
          attachTabs();
        }
        const focused = document.activeElement === tabs[1];
        select(0);
        tabs[1].disabled = true;
        frame.querySelector<HTMLElement>('.markdown-code-error')!.hidden = false;
        if (focused) tabs[0].focus();
      }
      async function start() {
        if (!nearby || selected !== 1 || started || tabs[1]?.disabled) return;
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
          image.addEventListener(
            'load',
            () => {
              rememberRenderedImageSize(sizeKey, image);
              renderPanel.removeAttribute('aria-busy');
            },
            {
              signal
            }
          );
          if (frame.dataset.artifact === 'svg') {
            image.className = 'markdown-svg-preview';
            image.alt = 'SVG';
            image.decoding = 'async';
            image.src = `data:image/svg+xml;charset=utf-8,${encodeURIComponent(source)}`;
          }
          const { RenderedImage } = await import('$lib/ui/attachments');
          if (signal.aborted || failed) return;
          rememberRenderedImageSize(sizeKey, image);
          const canvas = mount(RenderedImage, {
            target: renderPanel,
            props: { image, initialSize: size }
          });
          placeholder?.remove();
          disposeCanvas = () => {
            void unmount(canvas);
            disposeCanvas = undefined;
          };
          canvases.push(() => disposeCanvas?.());
          if (image.complete) {
            renderPanel.removeAttribute('aria-busy');
            if (!image.naturalWidth) fail();
          }
        } catch {
          fail();
        }
      }

      function attachTabs() {
        tabs.forEach((tab, i) => {
          const id = `${scope}-code-${index}-${i}`;
          tab.id = `${id}-tab`;
          tab.setAttribute('aria-controls', id);
          panels[i].id = id;
          panels[i].role = 'tabpanel';
          panels[i].setAttribute('aria-labelledby', tab.id);
          panels[i].tabIndex = 0;
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
      }
      if (!visual) attachTabs();
      if (tabs[1]?.disabled || frame.dataset.artifact === 'ansi') return;
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
