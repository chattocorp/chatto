import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import Toast from './Toast.svelte';
import type { ToastTone } from './toastState.svelte';

function toastShell(container: Element): HTMLElement {
  const shell = container.querySelector('.menu') as HTMLElement | null;
  if (!shell) throw new Error('Toast shell not found');
  return shell;
}

function dismissButton(container: Element): HTMLButtonElement {
  const button = container.querySelector<HTMLButtonElement>('button[aria-label]');
  if (!button) throw new Error('Toast dismiss button not found');
  return button;
}

function actionButton(container: Element, label: string): HTMLButtonElement {
  const button = [...container.querySelectorAll<HTMLButtonElement>('button')].find(
    (candidate) => candidate.textContent?.trim() === label
  );
  if (!button) throw new Error(`Action button "${label}" not found`);
  return button;
}

function toastSection(container: Element): HTMLElement {
  const section = container.querySelector('.menu-section') as HTMLElement | null;
  if (!section) throw new Error('Toast menu section not found');
  return section;
}

describe('Toast', () => {
  it.each([
    ['error', 'text-error'],
    ['success', 'text-success'],
    ['info', 'text-action'],
    ['warning', 'text-warning']
  ] satisfies Array<[ToastTone, string]>)('renders %s with semantic tone color', (tone, color) => {
    const { container } = render(Toast, {
      props: {
        tone,
        message: `${tone} message`,
        onDismiss: vi.fn()
      }
    });

    const icon = container.querySelector('.iconify');
    expect(icon).not.toBeNull();
    expect(icon?.classList.contains(color)).toBe(true);
    expect(toastShell(container).classList.contains('menu')).toBe(true);
    expect(toastSection(container).classList.contains('menu-section')).toBe(true);
  });

  it('renders message and compact action styling', async () => {
    const onClick = vi.fn();
    const { container } = render(Toast, {
      props: {
        tone: 'info',
        message: 'A new version is available',
        action: { label: 'Reload', onClick },
        onDismiss: vi.fn()
      }
    });

    await expect.element(toastShell(container)).toHaveTextContent('A new version is available');
    await expect.element(actionButton(container, 'Reload')).toHaveClass('btn-secondary');
    await expect.element(actionButton(container, 'Reload')).toHaveClass('btn-xs');
  });

  it('dismisses when clicked', () => {
    const onDismiss = vi.fn();
    const { container } = render(Toast, {
      props: {
        tone: 'success',
        message: 'Saved',
        onDismiss
      }
    });

    dismissButton(container).click();

    expect(onDismiss).toHaveBeenCalledOnce();
  });

  it('renders dismissal as a native button', () => {
    const { container } = render(Toast, {
      props: {
        tone: 'warning',
        message: 'Check your input',
        onDismiss: vi.fn()
      }
    });

    expect(dismissButton(container).tagName).toBe('BUTTON');
    expect(dismissButton(container).type).toBe('button');
  });

  it('runs action and dismisses once when action is clicked', () => {
    const onClick = vi.fn();
    const onDismiss = vi.fn();
    const { container } = render(Toast, {
      props: {
        tone: 'info',
        message: 'A new version is available',
        action: { label: 'Reload', onClick },
        onDismiss
      }
    });

    actionButton(container, 'Reload').click();

    expect(onClick).toHaveBeenCalledOnce();
    expect(onDismiss).toHaveBeenCalledOnce();
  });
});
