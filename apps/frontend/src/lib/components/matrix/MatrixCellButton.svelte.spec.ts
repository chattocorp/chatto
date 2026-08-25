import '../../../app.css';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import MatrixCellButton from './MatrixCellButton.svelte';

const baseProps = {
  icon: 'icon-[uil--check]',
  ariaLabel: 'Example matrix cell',
  onActivate: vi.fn()
};

describe('MatrixCellButton', () => {
  it('renders an explicit tone and activates through the native button', () => {
    const onActivate = vi.fn();
    const { container } = render(MatrixCellButton, {
      props: { ...baseProps, tone: 'success', explicit: true, pressed: true, onActivate }
    });
    const button = container.querySelector('button')!;

    expect(button.getAttribute('aria-label')).toBe('Example matrix cell');
    expect(button.getAttribute('aria-pressed')).toBe('true');
    expect(button.querySelector('[class~="bg-success"]')).not.toBeNull();
    expect(button.querySelector('[class~="icon-[uil--check]"]')).not.toBeNull();

    button.click();
    expect(onActivate).toHaveBeenCalledOnce();
  });

  it('renders inherited intensity and its non-colour marker', () => {
    const { container } = render(MatrixCellButton, {
      props: { ...baseProps, tone: 'warning', inheritedMarker: true }
    });

    expect(container.querySelector('[class~="bg-warning/20"]')).not.toBeNull();
    expect(container.querySelector('[class~="icon-[uil--link]"]')).not.toBeNull();
  });

  it('renders a bare full-size icon and fades inherited values', async () => {
    const rendered = render(MatrixCellButton, {
      props: { ...baseProps, tone: 'warning', explicit: true, variant: 'icon' }
    });

    const icon = rendered.container.querySelector('[class~="icon-[uil--check]"]')!;
    const iconShell = icon.parentElement!;
    expect(icon.className).toContain('h-5');
    expect(iconShell.className).toContain('text-warning');
    expect(iconShell.className).not.toContain('bg-warning');
    expect(iconShell.className).not.toContain('rounded-md');

    await rendered.rerender({
      ...baseProps,
      tone: 'warning',
      explicit: false,
      variant: 'icon'
    });
    expect(iconShell.className).toContain('opacity-40');
  });

  it('keeps the native button focusable but suppresses activation while loading', () => {
    const onActivate = vi.fn();
    const { container } = render(MatrixCellButton, {
      props: { ...baseProps, loading: true, onActivate }
    });
    const button = container.querySelector('button') as HTMLButtonElement;

    expect(button.disabled).toBe(false);
    expect(button.getAttribute('aria-busy')).toBe('true');
    expect(button.getAttribute('aria-disabled')).toBe('true');
    expect(button.querySelector('[class~="icon-[uil--spinner]"]')).not.toBeNull();

    button.click();
    expect(onActivate).not.toHaveBeenCalled();
  });

  it('uses native disabled semantics and gives a lock marker precedence', () => {
    const { container } = render(MatrixCellButton, {
      props: {
        ...baseProps,
        locked: true,
        warning: true,
        inheritedMarker: true
      }
    });
    const button = container.querySelector('button') as HTMLButtonElement;

    expect(button.disabled).toBe(true);
    expect(button.getAttribute('aria-disabled')).toBe('true');
    expect(button.querySelector('[class~="icon-[uil--lock]"]')).not.toBeNull();
    expect(button.querySelector('[class~="icon-[uil--exclamation-triangle]"]')).toBeNull();
    expect(button.querySelector('[class~="icon-[uil--link]"]')).toBeNull();
  });

  it('renders a warning marker for an editable constrained cell', () => {
    const { container } = render(MatrixCellButton, {
      props: { ...baseProps, warning: true }
    });
    expect(container.querySelector('[class~="icon-[uil--exclamation-triangle]"]')).not.toBeNull();
  });

  it('renders a labelled inert placeholder when a cell is not applicable', () => {
    const { container } = render(MatrixCellButton, {
      props: { ...baseProps, applicable: false, title: 'Not available at this scope' }
    });
    const placeholder = container.querySelector('[aria-label="Example matrix cell"]')!;

    expect(placeholder.tagName).toBe('SPAN');
    expect(placeholder.getAttribute('role')).toBe('img');
    expect(placeholder.textContent?.trim()).toBe('—');
    expect(placeholder.getAttribute('title')).toBe('Not available at this scope');
    expect(container.querySelector('button')).toBeNull();
  });

  it('maps every semantic tone to explicit and inherited surfaces', async () => {
    const rendered = render(MatrixCellButton, {
      props: { ...baseProps, tone: 'action', explicit: true }
    });
    expect(rendered.container.querySelector('[class~="bg-action"]')).not.toBeNull();

    await rendered.rerender({ ...baseProps, tone: 'danger', explicit: false });
    expect(rendered.container.querySelector('[class~="bg-danger/15"]')).not.toBeNull();

    await rendered.rerender({ ...baseProps, tone: 'neutral', explicit: true });
    expect(rendered.container.querySelector('[class~="bg-neutral-action"]')).not.toBeNull();
  });
});
