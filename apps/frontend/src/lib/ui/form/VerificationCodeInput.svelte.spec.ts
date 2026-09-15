import '../../../app.css';
import { flushSync } from 'svelte';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import VerificationCodeInput from './VerificationCodeInput.svelte';

const props = {
  label: 'Verification code',
  digitLabel: (number: number) => `Digit ${number}`
};

function inputValues(container: Element): string[] {
  return [...container.querySelectorAll<HTMLInputElement>('input')].map((input) => input.value);
}

describe('VerificationCodeInput', () => {
  it('keeps later digits in place while a middle digit is replaced', () => {
    const { container } = render(VerificationCodeInput, {
      props: { ...props, value: '123456' }
    });
    const inputs = [...container.querySelectorAll<HTMLInputElement>('input')];
    const editedInput = inputs[2];

    editedInput.focus();
    editedInput.value = '';
    editedInput.dispatchEvent(
      new InputEvent('input', { bubbles: true, inputType: 'deleteContentBackward' })
    );
    flushSync();

    expect(inputValues(container)).toEqual(['1', '2', '', '4', '5', '6']);
    expect(document.activeElement).toBe(editedInput);

    editedInput.value = '9';
    editedInput.dispatchEvent(
      new InputEvent('input', { bubbles: true, data: '9', inputType: 'insertText' })
    );
    flushSync();

    expect(inputValues(container)).toEqual(['1', '2', '9', '4', '5', '6']);
  });

  it('accepts an external reset after editing', async () => {
    const rendered = render(VerificationCodeInput, {
      props: { ...props, value: '123456' }
    });

    await rendered.rerender({ ...props, value: '' });
    flushSync();

    expect(inputValues(rendered.container)).toEqual(['', '', '', '', '', '']);
  });
});
