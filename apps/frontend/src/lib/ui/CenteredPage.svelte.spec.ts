import '../../app.css';
import { expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { testSnippet } from '$lib/test-utils';
import CenteredPage from './CenteredPage.svelte';

/** Render inside a bounded pane to exercise real browser layout and overflow. */
function renderPage(width: number, height: number, contentHeight: number) {
  const view = render(CenteredPage, {
    props: {
      header: testSnippet('<header style="height: 48px">Page navigation</header>'),
      children: testSnippet(
        `<div data-testid="content" style="height: ${contentHeight}px">Page content</div>`
      )
    }
  });
  Object.assign(view.container.style, {
    display: 'flex',
    width: `${width}px`,
    height: `${height}px`
  });
  return {
    content: view.container.querySelector<HTMLElement>('[data-testid="content"]')!,
    viewport: view.container.querySelector<HTMLElement>('[role="region"]')!,
    header: view.container.querySelector<HTMLElement>('header')!
  };
}

it('centers content on both axes below the fixed header', () => {
  const { content, viewport } = renderPage(1600, 900, 200);
  const body = content.getBoundingClientRect();
  const area = viewport.getBoundingClientRect();
  expect(Math.abs(body.x + body.width / 2 - (area.x + area.width / 2))).toBeLessThan(1);
  expect(Math.abs(body.y + body.height / 2 - (area.y + area.height / 2))).toBeLessThan(1);
  expect(body.width).toBeLessThan(area.width);
});

it('keeps both ends of tall content reachable without scrolling the header', () => {
  const { content, viewport, header } = renderPage(320, 300, 700);
  const headerTop = header.getBoundingClientRect().top;
  expect(content.getBoundingClientRect().top).toBeGreaterThanOrEqual(
    viewport.getBoundingClientRect().top
  );
  expect(viewport.scrollHeight).toBeGreaterThan(viewport.clientHeight);
  expect(viewport.scrollWidth).toBe(viewport.clientWidth);
  viewport.scrollTop = viewport.scrollHeight;
  expect(content.getBoundingClientRect().bottom).toBeLessThanOrEqual(
    viewport.getBoundingClientRect().bottom
  );
  expect(header.getBoundingClientRect().top).toBe(headerTop);
});
