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

  it('shows the allow icon when override is allow', async () => {
    const { container } = renderCell({ override: 'allow' });
    expect(container.querySelector('.uil--check')).not.toBeNull();
    expect(container.querySelector('.uil--times')).toBeNull();
  });

  it('shows the deny icon when override is deny', async () => {
    const { container } = renderCell({ override: 'deny' });
    expect(container.querySelector('.uil--times')).not.toBeNull();
    expect(container.querySelector('.uil--check')).toBeNull();
  });

  it('shows the inherited icon when there is no override', async () => {
    const { container } = renderCell({ override: 'neutral', inherited: 'allow' });
    // Effective visual state is the inherited baseline when no override.
    expect(container.querySelector('.uil--check')).not.toBeNull();
    // But the cell is not "pressed" — it's a faded inherited cell.
    expect(container.querySelector('button')!.getAttribute('aria-pressed')).toBe('false');
  });
});
