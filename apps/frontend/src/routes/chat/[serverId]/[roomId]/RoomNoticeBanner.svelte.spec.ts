import '../../../../app.css';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import { RoomNotice } from '@chatto/api-types/api/v1/rooms_pb';
import { RoomNoticeStore } from '@chatto/client/room/notice';
import type { ServerConnection } from '@chatto/client/server/serverConnection';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { setReactiveLocale } from '$lib/i18n/state.svelte';
import RoomNoticeBanner from './RoomNoticeBanner.svelte';
import { Timestamp } from '@bufbuild/protobuf';
import { tick } from 'svelte';

describe('RoomNoticeBanner', () => {
  beforeEach(async () => {
    localStorage.clear();
    await loadLocaleMessages('en-GB');
    setReactiveLocale('en-GB');
  });

  it('keeps the same banner mounted when Shield Mode changes editing permission', async () => {
    const notice = new RoomNotice({ body: 'Stable notice', publicationId: 'p1' });
    const store = new RoomNoticeStore(
      { getAPI: () => ({ get: async () => notice }) } as unknown as ServerConnection,
      'R1'
    );
    await store.refresh();
    const view = render(RoomNoticeBanner, {
      store,
      serverId: 'test-server',
      viewerId: 'viewer',
      canManage: false
    });
    const surface = view.container.querySelector('.origin-top')!;
    const dismiss = page.getByRole('button', { name: 'Dismiss', exact: true });
    for (const canManage of [true, false, true, false]) {
      await view.rerender({ canManage });
      expect(view.container.querySelectorAll('.origin-top')).toHaveLength(1);
      expect(view.container.querySelector('.origin-top')).toBe(surface);
      expect((surface as HTMLElement).style.position).not.toBe('absolute');
      await expect.element(dismiss).toBeVisible();
      if (canManage)
        await expect.element(page.getByRole('button', { name: 'Edit', exact: true })).toBeVisible();
      else
        await expect
          .element(page.getByRole('button', { name: 'Edit', exact: true }))
          .not.toBeInTheDocument();
    }
    await view.unmount();
    store.dispose();
  });

  it('shows publication time using the viewer timezone and clock preference', async () => {
    await page.viewport(1280, 720);
    const notice = new RoomNotice({
      body: 'Notice',
      publicationId: 'p1',
      publishedAt: Timestamp.fromDate(new Date('2026-10-26T13:05:00Z'))
    });
    const store = new RoomNoticeStore(
      { getAPI: () => ({ get: async () => notice }) } as unknown as ServerConnection,
      'R1'
    );
    await store.refresh();
    const view = render(RoomNoticeBanner, {
      store,
      serverId: 'test-server',
      viewerId: 'viewer',
      timeSettings: { effectiveTimezone: 'Europe/Berlin', effectiveHour12: false }
    });
    await expect
      .element(view.container.querySelector('time')!)
      .toHaveAttribute('datetime', '2026-10-26T13:05:00.000Z');
    await expect.element(view.container.querySelector('time')!).toHaveTextContent('14:05');
    const time = view.container.querySelector('time')!;
    const header = view.container.querySelector('.panel-header')!;
    const button = header.querySelector('button')!;
    const headerHeight = header.getBoundingClientRect().height;
    button.style.display = 'none';
    expect(header.getBoundingClientRect().height).toBe(headerHeight);
    button.style.display = '';
    expect(time.textContent).not.toContain('Published');
    expect(time.title).toBe(time.textContent?.trim());
    view.container.style.width = '220px';
    await vi.waitFor(() => {
      expect(time.scrollWidth).toBeGreaterThan(time.clientWidth);
      expect(getComputedStyle(time).textOverflow).toBe('ellipsis');
      expect(time.getBoundingClientRect().height).toBeLessThan(30);
      expect(view.container.scrollWidth).toBeLessThanOrEqual(220);
    });
    store.dispose();
  });
  it('uses the full width and keeps dismissal in the header above scrollable content', async () => {
    let current = new RoomNotice({
      body: Array.from({ length: 30 }, (_, i) => `Paragraph ${i}: Read this room notice.`).join(
        '\n\n'
      ),
      version: 'v1',
      publicationId: 'p1'
    });
    const store = new RoomNoticeStore(
      { getAPI: () => ({ get: async () => current }) } as unknown as ServerConnection,
      'R1'
    );
    await store.refresh();
    const host = document.createElement('div');
    host.style.cssText = 'display:flex;flex-direction:column;width:390px;height:500px';
    document.body.append(host);
    const view = render(RoomNoticeBanner, {
      target: host,
      props: { store, serverId: 'test-server', viewerId: 'viewer' }
    });
    const content = host.querySelector('.overflow-y-auto') as HTMLElement;
    const button = host.querySelector('button')!;
    const topFade = host.querySelector<HTMLElement>('.bg-gradient-to-b')!;
    const bottomFade = host.querySelector<HTMLElement>('.bg-gradient-to-t')!;
    try {
      const section = host.querySelector('section')!;
      for (const viewportHeight of [720, 1200]) {
        await page.viewport(1280, viewportHeight);
        await vi.waitFor(() => {
          expect(section.getBoundingClientRect().height).toBeLessThanOrEqual(
            Math.min(viewportHeight * 0.4, 384)
          );
          expect(content.scrollHeight).toBeGreaterThan(content.clientHeight);
        });
      }
      await page.viewport(390, 844);
      for (const height of [500, 250]) {
        host.style.height = `${height}px`;
        content.scrollTop = 0;
        await vi.waitFor(() => {
          expect(content.scrollHeight).toBeGreaterThan(content.clientHeight);
          expect(host.scrollHeight).toBeLessThanOrEqual(height);
          expect(section.getBoundingClientRect().height).toBeGreaterThanOrEqual(height - 1);
          expect(button.getBoundingClientRect().bottom).toBeLessThanOrEqual(
            host.getBoundingClientRect().bottom
          );
          expect(button.getBoundingClientRect().bottom).toBeLessThanOrEqual(
            content.getBoundingClientRect().top
          );
          expect(content.getBoundingClientRect().right).toBeGreaterThan(
            button.getBoundingClientRect().right
          );
        });
        await expect.element(topFade).toHaveClass('opacity-0');
        await expect.element(bottomFade).not.toHaveClass('opacity-0');
        content.scrollTop = 50;
        await expect.element(topFade).not.toHaveClass('opacity-0');
        await expect.element(bottomFade).not.toHaveClass('opacity-0');
        content.scrollTop = content.scrollHeight;
        expect(content.scrollTop).toBeGreaterThan(0);
        await expect.element(topFade).not.toHaveClass('opacity-0');
        await expect.element(bottomFade).toHaveClass('opacity-0');
      }
      current = new RoomNotice({ body: 'Short notice', publicationId: 'p1' });
      await store.refresh();
      await expect.element(topFade).toHaveClass('opacity-0');
      await expect.element(bottomFade).toHaveClass('opacity-0');
      const outgoing = host.querySelector('.origin-top') as HTMLElement;
      button.click();
      await tick();
      expect(outgoing.isConnected).toBe(true);
      expect(outgoing.style.position).toBe('absolute');
      expect(section.getBoundingClientRect().height).toBe(0);
      await expect
        .element(page.getByRole('button', { name: 'Show notice' }))
        .not.toBeInTheDocument();
    } finally {
      await page.viewport(1280, 720);
      await view.unmount();
      host.remove();
      store.dispose();
    }
  });
  it('keeps corrections dismissed, resurfaces republishing, and remembers dismissal after remount', async () => {
    let current = new RoomNotice({
      body: '**Important notice**',
      version: 'v1',
      publicationId: 'p1'
    });
    const store = new RoomNoticeStore(
      { getAPI: () => ({ get: async () => current }) } as unknown as ServerConnection,
      'R1'
    );
    await store.refresh();
    const props = { store, serverId: 'test-server', viewerId: 'viewer' };
    const view = render(RoomNoticeBanner, props);
    await expect.element(page.getByText('Important notice', { exact: true })).toBeVisible();
    await page.getByRole('button', { name: 'Dismiss', exact: true }).click();
    current = new RoomNotice({ body: 'Corrected notice', version: 'v2', publicationId: 'p1' });
    await store.refresh();
    await expect
      .element(page.getByText('Corrected notice', { exact: true }))
      .not.toBeInTheDocument();
    await view.unmount();
    const next = render(RoomNoticeBanner, props);
    await expect
      .element(page.getByRole('button', { name: 'Show notice', exact: true }))
      .not.toBeInTheDocument();
    expect(next.container.querySelector('section')!.getBoundingClientRect().height).toBe(0);
    current = new RoomNotice({ body: 'Republished notice', version: 'v3', publicationId: 'p3' });
    await store.refresh();
    await expect.element(page.getByText('Republished notice', { exact: true })).toBeVisible();
    await page.getByRole('button', { name: 'Dismiss', exact: true }).click();
    await expect
      .element(page.getByText('Republished notice', { exact: true }))
      .not.toBeInTheDocument();
    await next.rerender({ viewerId: 'other-viewer' });
    await expect.element(page.getByText('Republished notice', { exact: true })).toBeVisible();
    store.dispose();
  });
});
