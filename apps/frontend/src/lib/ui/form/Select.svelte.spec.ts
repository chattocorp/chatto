import '../../../app.css';
import { userEvent } from 'vitest/browser';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import Select from './Select.svelte';
import SelectTestHarness from './SelectTestHarness.svelte';

const options = [
  { value: 'first', label: 'First device' },
  { value: 'second', label: 'Second device' }
];
const props = { id: 'device', label: 'Device', options, value: 'first' };

describe('Select', () => {
  it('updates a bound value through native selection', async () => {
    const screen = render(SelectTestHarness);
    await screen.getByRole('combobox').selectOptions('second');
    await expect.element(screen.getByRole('status')).toHaveTextContent('second');
  });

  it('updates a bound value through a real picker click', async () => {
    const screen = render(SelectTestHarness);
    const select = screen.getByRole('combobox');
    await select.click();
    await select.getByRole('option', { name: 'Second device' }).click();
    await expect.element(select).toHaveValue('second');
    await expect.element(screen.getByRole('status')).toHaveTextContent('second');
  });

  it('supports native keyboard selection and focus', async () => {
    const screen = render(SelectTestHarness);
    const select = screen.getByRole('combobox');
    await select.click();
    await userEvent.keyboard('{ArrowDown}{Enter}');
    await expect.element(select).toHaveValue('second');
    await expect.element(select).toHaveFocus();
    await expect.element(screen.getByRole('status')).toHaveTextContent('second');
  });

  it('keeps native selection working without the enhanced appearance', async () => {
    const screen = render(SelectTestHarness);
    const select = screen.getByRole('combobox');
    (select.element() as HTMLSelectElement).style.appearance = 'auto';
    await select.selectOptions('second');
    await expect.element(screen.getByRole('status')).toHaveTextContent('second');
  });

  it('does not take focus from another control after a pending change', async () => {
    let finish!: () => void;
    const screen = render(Select, {
      ...props,
      onValueChange: () =>
        new Promise<void>((resolve) => {
          finish = resolve;
        })
    });
    const select = screen.getByRole('combobox');
    const other = document.createElement('button');
    screen.container.append(other);
    (select.element() as HTMLSelectElement).focus();
    await select.selectOptions('second');
    other.focus();
    finish();
    await expect.element(select).toBeEnabled();
    expect(document.activeElement).toBe(other);
  });

  it('blocks overlapping changes and displays the committed value after success', async () => {
    let finish!: () => void;
    const onValueChange = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          finish = resolve;
        })
    );
    const screen = render(Select, { ...props, onValueChange });
    const select = screen.getByRole('combobox', { name: 'Device' });
    (select.element() as HTMLSelectElement).focus();
    await select.selectOptions('second');
    await expect.element(select).toBeDisabled();
    await expect.element(select).toHaveValue('first');
    expect(onValueChange).toHaveBeenCalledExactlyOnceWith('second');
    // Even a synthetic change cannot start a second operation while pending.
    select.element().dispatchEvent(new Event('change', { bubbles: true }));
    expect(onValueChange).toHaveBeenCalledOnce();
    await screen.rerender({ value: 'second' });
    finish();
    await expect.element(select).toBeEnabled();
    await expect.element(select).toHaveValue('second');
    await expect.element(select).toHaveFocus();
  });

  it.each(['unchanged', 'rejected'])(
    'restores the owner value when a change is %s',
    async (failure) => {
      const screen = render(Select, {
        ...props,
        onValueChange: async () => {
          if (failure === 'rejected') throw new Error('Device switch failed');
        }
      });
      const select = screen.getByRole('combobox');
      await select.selectOptions('second');
      await expect.element(select).toBeEnabled();
      await expect.element(select).toHaveValue('first');
    }
  );

  it('keeps native required validation and associates error text', async () => {
    const screen = render(Select, {
      ...props,
      value: '',
      placeholder: 'Choose a device',
      required: true,
      error: 'Select a device to continue.'
    });
    const select = screen.getByRole('combobox');
    expect((select.element() as HTMLSelectElement).validity.valueMissing).toBe(true);
    await expect.element(select).toHaveAttribute('aria-describedby', 'device-error');
    await expect.element(select).toHaveAttribute('aria-invalid', 'true');
    await select.selectOptions('first');
    expect((select.element() as HTMLSelectElement).validity.valid).toBe(true);
  });

  it('preserves external disabled state and helper text', async () => {
    const screen = render(Select, {
      ...props,
      disabled: true,
      description: 'Output is unavailable.'
    });
    await expect.element(screen.getByRole('combobox')).toBeDisabled();
    await expect
      .element(screen.getByRole('combobox'))
      .toHaveAttribute('aria-describedby', 'device-description');
  });
});
