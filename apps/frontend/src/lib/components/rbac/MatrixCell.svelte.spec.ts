import { describe, it, expect, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import MatrixCell from './MatrixCell.svelte';

type State = 'allow' | 'deny' | 'neutral';

function renderCell(
  props: Partial<{
    override: State;
    inherited: State;
    applicable: boolean;
    disabled: boolean;
    locked: boolean;
    allowBlocked: boolean;
    ceilingBlocked: boolean;
    decisionMode: 'tri-state' | 'binary';
    updating: boolean;
    ariaLabel: string;
    title: string;
    onCycle: (next: State) => void;
  }>
) {
  return render(MatrixCell, {
    props: {
      override: 'neutral',
      inherited: 'neutral',
      applicable: true,
      disabled: false,
      updating: false,
      ariaLabel: 'cell',
      onCycle: vi.fn(),
      ...props
    }
  });
}

describe('MatrixCell', () => {
  it('renders an inert "—" when not applicable', async () => {
    const { container } = renderCell({ applicable: false, ariaLabel: 'inert cell' });
    const cell = container.querySelector('[aria-label="inert cell"]') as HTMLElement;
    expect(cell?.tagName).toBe('SPAN');
    expect(cell?.textContent?.trim()).toBe('—');
  });

  it('cycles neutral → allow on click', async () => {
    const onCycle = vi.fn();
    const { container } = renderCell({ override: 'neutral', onCycle });
    const button = container.querySelector('button') as HTMLButtonElement;
    button.click();
    flushSync();
    expect(onCycle).toHaveBeenCalledWith('allow');
  });

  it('skips allow when an external permission ceiling blocks it', async () => {
    const onCycle = vi.fn();
    const { container } = renderCell({ override: 'neutral', allowBlocked: true, onCycle });
    container.querySelector('button')!.click();
    flushSync();
    expect(onCycle).toHaveBeenCalledWith('deny');
  });

  it('cycles allow → deny on click', async () => {
    const onCycle = vi.fn();
    const { container } = renderCell({ override: 'allow', onCycle });
    container.querySelector('button')!.click();
    flushSync();
    expect(onCycle).toHaveBeenCalledWith('deny');
  });

  it('cycles deny → neutral on click', async () => {
    const onCycle = vi.fn();
    const { container } = renderCell({ override: 'deny', onCycle });
    container.querySelector('button')!.click();
    flushSync();
    expect(onCycle).toHaveBeenCalledWith('neutral');
  });

  it('reflects override state in aria-pressed', async () => {
    const { container, rerender } = renderCell({ override: 'neutral' });
    let button = container.querySelector('button') as HTMLButtonElement;
    expect(button.getAttribute('aria-pressed')).toBe('false');

    await rerender({
      override: 'allow',
      inherited: 'neutral',
      applicable: true,
      disabled: false,
      updating: false,
      ariaLabel: 'cell',
      onCycle: vi.fn()
    });
    button = container.querySelector('button') as HTMLButtonElement;
    expect(button.getAttribute('aria-pressed')).toBe('true');
  });

  it('does not call onCycle when disabled', async () => {
    const onCycle = vi.fn();
    const { container } = renderCell({ disabled: true, onCycle });
    container.querySelector('button')!.click();
    flushSync();
    expect(onCycle).not.toHaveBeenCalled();
  });

  it('shows a spinner and prevents repeat clicks while updating', async () => {
    const onCycle = vi.fn();
    const { container } = renderCell({ updating: true, onCycle });
    const button = container.querySelector('button') as HTMLButtonElement;

    expect(button.disabled).toBe(true);
    expect(button.getAttribute('aria-busy')).toBe('true');
    expect(button.className).toContain('ring-action/40');
    expect(
      button.querySelector('.h-4.w-4.animate-spin[class~="icon-[uil--spinner]"]')
    ).not.toBeNull();
    expect(button.querySelector('[class~="icon-[uil--minus]"]')).toBeNull();

    button.click();
    flushSync();
    expect(onCycle).not.toHaveBeenCalled();
  });

  it('shows the allow icon when override is allow', async () => {
    const { container } = renderCell({ override: 'allow' });
    expect(container.querySelector('[class~="icon-[uil--check]"]')).not.toBeNull();
    expect(container.querySelector('[class~="icon-[uil--times]"]')).toBeNull();
  });

  it('shows the deny icon when override is deny', async () => {
    const { container } = renderCell({ override: 'deny' });
    expect(container.querySelector('[class~="icon-[uil--times]"]')).not.toBeNull();
    expect(container.querySelector('[class~="icon-[uil--check]"]')).toBeNull();
  });

  it('shows the inherited icon when there is no override', async () => {
    const { container } = renderCell({ override: 'neutral', inherited: 'allow' });
    // Effective visual state is the inherited baseline when no override.
    expect(container.querySelector('[class~="icon-[uil--check]"]')).not.toBeNull();
    // But the cell is not "pressed" — it's a faded inherited cell.
    expect(container.querySelector('button')!.getAttribute('aria-pressed')).toBe('false');
  });

  it('marks configured bot allows blocked by the owner ceiling', () => {
    const { container } = renderCell({ override: 'allow', ceilingBlocked: true });
    const button = container.querySelector('button')!;

    expect(button.className).not.toContain('ring-warning');
    expect(button.querySelector('[class~="bg-warning"]')).not.toBeNull();
    expect(button.querySelector('[class~="bg-success"]')).toBeNull();
    expect(button.className).toContain('cursor-pointer');
    expect(button.querySelector('[class~="icon-[uil--exclamation-triangle]"]')).not.toBeNull();
    expect(button.querySelector('[class~="icon-[uil--lock]"]')).toBeNull();
  });

  it('uses a quiet warning state for a blocked inherited allow', () => {
    const { container } = renderCell({ inherited: 'allow', ceilingBlocked: true });
    const button = container.querySelector('button')!;

    expect(button.querySelector('[class~="bg-warning/20"]')).not.toBeNull();
    expect(button.querySelector('[class~="bg-success/15"]')).toBeNull();
    expect(button.querySelector('[class~="icon-[uil--check]"]')).not.toBeNull();
    expect(button.querySelector('[class~="icon-[uil--exclamation-triangle]"]')).not.toBeNull();
    expect(button.querySelector('[class~="icon-[uil--lock]"]')).toBeNull();
  });

  it('makes a permission the owner cannot grant visibly and functionally inert', () => {
    const { container } = renderCell({ decisionMode: 'binary', allowBlocked: true });
    const button = container.querySelector('button')!;
    const surface = button.firstElementChild!;

    expect(button.hasAttribute('disabled')).toBe(true);
    expect(button.className).toContain('cursor-not-allowed');
    expect(button.className).not.toContain('cursor-pointer');
    expect(button.className).not.toContain('active:scale-[0.96]');
    expect(button.className).not.toContain('opacity-60');
    expect(surface.className).not.toContain('hover:');
    expect(button.querySelector('[class~="icon-[uil--lock]"]')).not.toBeNull();
    expect(button.querySelector('[class~="icon-[uil--exclamation-triangle]"]')).toBeNull();
    expect(button.querySelector('[class~="icon-[uil--minus]"]')).not.toBeNull();
  });

  it('uses a warning marker when a constrained tri-state cell remains editable', () => {
    const { container } = renderCell({ allowBlocked: true });
    const button = container.querySelector('button')!;

    expect(button.disabled).toBe(false);
    expect(button.className).toContain('cursor-pointer');
    expect(button.querySelector('[class~="icon-[uil--exclamation-triangle]"]')).not.toBeNull();
    expect(button.querySelector('[class~="icon-[uil--lock]"]')).toBeNull();
  });

  it('keeps a locked inherited grant visible but non-interactive', () => {
    const onCycle = vi.fn();
    const { container } = renderCell({
      inherited: 'allow',
      decisionMode: 'binary',
      locked: true,
      onCycle
    });
    const button = container.querySelector('button')!;

    expect(button.disabled).toBe(true);
    expect(button.className).toContain('cursor-not-allowed');
    expect(button.className).not.toContain('cursor-pointer');
    expect(button.className).not.toContain('opacity-60');
    expect(button.firstElementChild!.className).not.toContain('hover:');
    expect(button.querySelector('[class~="bg-success/15"]')).not.toBeNull();
    expect(button.querySelector('[class~="icon-[uil--lock]"]')).not.toBeNull();

    button.click();
    expect(onCycle).not.toHaveBeenCalled();
  });

  it('toggles only enabled and disabled in binary mode', () => {
    const disable = vi.fn();
    const enabled = renderCell({
      inherited: 'allow',
      decisionMode: 'binary',
      onCycle: disable
    });
    enabled.container.querySelector('button')!.click();
    expect(disable).toHaveBeenCalledWith('neutral');
    expect(enabled.container.querySelector('button')!.getAttribute('aria-pressed')).toBe('true');

    const enable = vi.fn();
    const disabled = renderCell({
      override: 'neutral',
      decisionMode: 'binary',
      onCycle: enable
    });
    disabled.container.querySelector('button')!.click();
    expect(enable).toHaveBeenCalledWith('allow');
    expect(disabled.container.querySelector('[class~="icon-[uil--times]"]')).toBeNull();
  });

  it('disables a blocked binary grant while allowing a dormant grant to be removed', () => {
    const onCycle = vi.fn();
    const disabled = renderCell({
      decisionMode: 'binary',
      allowBlocked: true,
      onCycle
    });
    expect(disabled.container.querySelector('button')!.hasAttribute('disabled')).toBe(true);

    const dormant = renderCell({
      override: 'allow',
      decisionMode: 'binary',
      allowBlocked: true,
      ceilingBlocked: true,
      onCycle
    });
    dormant.container.querySelector('button')!.click();
    expect(onCycle).toHaveBeenCalledWith('neutral');
  });
});
