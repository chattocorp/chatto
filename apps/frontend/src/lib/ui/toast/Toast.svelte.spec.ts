import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import Toast from './Toast.svelte';
import type { ToastTone } from './toastState.svelte';

function toastButton(container: Element): HTMLElement {
  const button = container.querySelector('[role="button"]') as HTMLElement | null;
  if (!button) throw new Error('Toast button not found');
  return button;
}

function actionButton(container: Element, label: string): HTMLButtonElement {
  const button = [...container.querySelectorAll<HTMLButtonElement>('button')].find(
    (candidate) => candidate.textContent?.trim() === label
  );
  if (!button) throw new Error(`Action button "${label}" not found`);
  return button;
}

describe('Toast', () => {
  it.each([
    ['error', 'text-error'],
    ['success', 'text-success'],
    ['info', 'text-accent'],
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
    expect(toastButton(container).classList.contains('bg-surface-100')).toBe(true);
    expect(toastButton(container).classList.contains('border-text/10')).toBe(true);
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

    await expect.element(toastButton(container)).toHaveTextContent('A new version is available');
    await expect.element(actionButton(container, 'Reload')).toHaveClass('btn-secondary');
    await expect.element(actionButton(container, 'Reload')).toHaveClass('h-8');
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

    toastButton(container).click();

    expect(onDismiss).toHaveBeenCalledOnce();
  });

  it('dismisses from keyboard activation', () => {
    const onDismiss = vi.fn();
    const { container } = render(Toast, {
      props: {
        tone: 'warning',
        message: 'Check your input',
        onDismiss
      }
    });

    toastButton(container).dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }));
    toastButton(container).dispatchEvent(new KeyboardEvent('keydown', { key: ' ', bubbles: true }));

    expect(onDismiss).toHaveBeenCalledTimes(2);
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
