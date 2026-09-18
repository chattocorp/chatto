import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { createRawSnippet } from 'svelte';
import { q } from '$lib/test-utils';
import PaneHeader from './PaneHeader.svelte';
import '../../app.css';
import { page } from 'vitest/browser';

const actions = createRawSnippet(() => ({
  render: () => '<div><button type="button" data-testid="members">Members</button><button type="button">Call</button></div>'
}));
const collapsedActions = createRawSnippet(() => ({
  render: () => '<button type="button" data-testid="active-call">Active call</button>'
}));

describe('PaneHeader responsive actions', () => {
  it('reclaims only opted-in header space on mobile and restores it on close', async () => {
    await page.viewport(390, 800);
    const optedIn = render(PaneHeader, { props: { title: 'Room', hideOnKeyboard: true } });
    const normal = render(PaneHeader, { props: { title: 'Thread', onBack: () => {} } });
    const header = optedIn.container.firstElementChild!;
    const height = header.getBoundingClientRect().height;
    expect(height).toBeGreaterThan(0);
    try {
      document.body.setAttribute('data-keyboard-open', '');
      await expect.element(q(optedIn.container, 'h1')).not.toBeVisible();
      expect(header.getBoundingClientRect().height).toBe(0);
      await expect.element(normal.getByRole('button', { name: 'Back' })).toBeVisible();

      await page.viewport(1024, 800);
      await expect.element(optedIn.getByRole('heading', { name: 'Room' })).toBeVisible();
      await page.viewport(390, 800);
      document.body.removeAttribute('data-keyboard-open');
      await expect.element(optedIn.getByRole('heading', { name: 'Room' })).toBeVisible();
      expect(header.getBoundingClientRect().height).toBe(height);
    } finally {
      document.body.removeAttribute('data-keyboard-open');
      await page.viewport(1280, 720);
    }
  });

  it('expands inline and restores focus on Escape', async () => {
    const { container, getByRole } = render(PaneHeader, {
      props: { title: 'General', collapseActions: true, actions, collapsedActions }
    });
    container.style.width = '390px';

    const toggle = getByRole('button', { name: 'Pane actions' });
    await expect.element(toggle).toHaveAttribute('aria-expanded', 'false');
    await expect.element(getByRole('button', { name: 'Active call' })).toBeVisible();
    await expect.element(q(container, '[data-testid="members"]')).not.toBeVisible();
    const header = container.querySelector('[data-page-reveal]')!;
    const height = header.getBoundingClientRect().height;

    await toggle.click();
    await expect.element(getByRole('button', { name: 'Members' })).toBeVisible();
    await expect.element(getByRole('button', { name: 'Active call' })).not.toBeInTheDocument();
    expect(header.getBoundingClientRect().height).toBe(height);

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', cancelable: true }));
    await expect.element(toggle).toHaveAttribute('aria-expanded', 'false');
    await expect.element(toggle).toHaveFocus();
  });

  it('uses pane width and keeps important actions out of the wide toolbar', async () => {
    const { container, getByRole } = render(PaneHeader, {
      props: { title: 'General', collapseActions: true, actions, collapsedActions }
    });
    container.style.width = '600px';
    await expect.element(getByRole('button', { name: 'Members' })).toBeVisible();
    await expect.element(q(container, '[aria-controls]')).not.toBeVisible();
    await expect.element(q(container, '[data-testid="active-call"]')).not.toBeVisible();

    container.style.width = '390px';
    await expect.element(getByRole('button', { name: 'Pane actions' })).toBeVisible();
    await expect.element(q(container, '[data-testid="members"]')).not.toBeVisible();
    await expect.element(getByRole('button', { name: 'Active call' })).toBeVisible();
  });

  it('keeps actions inline unless the consumer enables collapse', async () => {
    const { container, getByRole } = render(PaneHeader, {
      props: { title: 'General', actions }
    });
    container.style.width = '390px';
    await expect.element(getByRole('button', { name: 'Members' })).toBeVisible();
    await expect.element(getByRole('button', { name: 'Pane actions' })).not.toBeInTheDocument();
  });
});
