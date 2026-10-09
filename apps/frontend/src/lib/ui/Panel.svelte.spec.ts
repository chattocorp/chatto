import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { testSnippet } from '$lib/test-utils';
import Panel from './Panel.svelte';
import { cdp, page } from 'vitest/browser';
import '../../app.css';

function renderPanel(noPadding = false) {
  return render(Panel, {
    props: {
      title: 'General',
      noPadding,
      children: testSnippet('<div data-testid="content">Content</div>')
    }
  });
}

describe('Panel inset structure', () => {
  it('flattens page panels on narrow touch and mouse screens while standalone panels keep their frames', async () => {
    const flat = render(Panel, {
      props: {
        title: 'Search query',
        titleHiddenOnNarrow: true,
        children: testSnippet('<div>Search controls</div>')
      }
    });
    flat.container.classList.add('pane-content');
    const normal = renderPanel();
    const inDialog = renderPanel();
    inDialog.container.classList.add('pane-content');
    const dialogShell = inDialog.container.firstElementChild as HTMLElement;
    // Native dialogs remain DOM descendants of their declaring page.
    const dialog = document.createElement('dialog');
    dialog.open = true;
    inDialog.container.append(dialog);
    dialog.append(dialogShell);
    const shell = flat.container.firstElementChild as HTMLElement;
    const inset = shell.lastElementChild!.firstElementChild as HTMLElement;
    const heading = shell.querySelector('h2')!;
    try {
      for (const touch of [false, true]) {
        await cdp().send('Emulation.setTouchEmulationEnabled', { enabled: touch });
        await page.viewport(390, 800);
        expect(getComputedStyle(shell).borderLeftWidth).toBe('0px');
        expect(getComputedStyle(shell).borderRadius).toBe('0px');
        expect(getComputedStyle(shell).boxShadow).toBe('none');
        expect(getComputedStyle(shell).backgroundImage).toBe('none');
        expect(getComputedStyle(inset).boxShadow).toBe('none');
        expect(getComputedStyle(inset).outlineStyle).toBe('none');
        expect(getComputedStyle(inset).paddingLeft).toBe('16px');
        expect(getComputedStyle(shell).marginLeft).toBe('-16px');
        expect(getComputedStyle(heading).position).toBe('absolute');
        expect(getComputedStyle(normal.container.firstElementChild!).borderLeftWidth).toBe('1px');
        expect(getComputedStyle(dialogShell).borderLeftWidth).toBe('1px');
        expect(getComputedStyle(dialogShell).marginLeft).toBe('0px');
      }
      await page.viewport(768, 800);
      expect(getComputedStyle(shell).borderLeftWidth).toBe('1px');
      expect(getComputedStyle(shell).marginLeft).toBe('0px');
      expect(getComputedStyle(shell).borderRadius).not.toBe('0px');
      expect(getComputedStyle(inset).paddingLeft).toBe('20px');
      expect(getComputedStyle(heading).position).toBe('static');
    } finally {
      await cdp().send('Emulation.setTouchEmulationEnabled', { enabled: false });
      await page.viewport(1280, 720);
    }
  });

  it('wraps padded content in the shared surface frame and rounded work plane', async () => {
    const { container } = renderPanel();
    const shell = container.querySelector('.panel-shell') as HTMLElement;
    const header = shell.querySelector(':scope > .panel-header') as HTMLElement;
    const frame = shell.querySelector(':scope > div:last-child') as HTMLElement;
    const inset = frame.firstElementChild as HTMLElement;

    expect(shell.className).toContain('overflow-hidden');
    expect(shell.className).toContain('shrink-0');
    expect(header.className).toContain('px-6');
    expect(header.className).toContain('py-3');
    expect(frame.className).toContain('px-1');
    expect(frame.className).toContain('pb-1');
    expect(frame.className).not.toContain('pt-1');
    expect(inset.className).toContain('panel-inset');
    expect(inset.className).toContain('p-5');
    expect(header.className).toContain('panel-header');
  });

  it('clips edge-to-edge content inside the same rounded work plane', async () => {
    const { container } = renderPanel(true);
    const inset = container.querySelector('.panel-shell > div:last-child > div') as HTMLElement;

    expect(inset.className).toContain('panel-inset-flush');
    expect(inset.className).not.toContain('p-5');
  });

  it('does not stack top frame spacing above edge-to-edge table content', async () => {
    const { container } = render(Panel, {
      props: {
        noPadding: true,
        children: testSnippet('<div>Content</div>')
      }
    });
    const frame = container.querySelector('.panel-shell > div') as HTMLElement;

    expect(frame.className).toContain('px-1');
    expect(frame.className).toContain('pb-1');
    expect(frame.className).not.toContain('pt-1');
  });

  it('keeps a full frame around untitled padded content', async () => {
    const { container } = render(Panel, {
      props: {
        children: testSnippet('<div>Content</div>')
      }
    });
    const frame = container.querySelector('.panel-shell > div') as HTMLElement;

    expect(frame.className).toContain('p-1');
  });

  it('renders a rich subtitle snippet', async () => {
    const { container } = render(Panel, {
      props: {
        title: 'General',
        subtitle: testSnippet('<a data-testid="subtitle-link" href="/rooms">Manage rooms</a>'),
        children: testSnippet('<div>Content</div>')
      }
    });

    expect(container.querySelector('[data-testid="subtitle-link"]')?.getAttribute('href')).toBe(
      '/rooms'
    );
  });

  it('can fill a flex page while preserving its inset structure', () => {
    const { container } = render(Panel, {
      props: { fillHeight: true, noPadding: true, children: testSnippet('<div>Content</div>') }
    });
    const shell = container.querySelector('.panel-shell') as HTMLElement;
    const frame = shell.querySelector(':scope > div') as HTMLElement;
    const inset = frame.firstElementChild as HTMLElement;

    expect(shell.className).toContain('flex-1');
    expect(frame.className).toContain('flex-1');
    expect(inset.className).toContain('flex-1');
  });
});
