import { describe, it, expect } from 'vitest';
import { z } from 'zod';
import { createFormState } from './formState.svelte';
import { flushSync } from 'svelte';

const schema = z.object({
  name: z.string().min(1, 'Name is required').max(10, 'Too long'),
  email: z.string().email('Invalid email')
});

type Values = z.infer<typeof schema>;

describe('createFormState', () => {
  it('hides field errors until the field is touched', () => {
    const form = createFormState<Values>(schema, { name: '', email: '' });
    expect(form.fieldError('name')).toBeUndefined();
    form.touch('name');
    expect(form.fieldError('name')).toBe('Name is required');
  });

  it('reflects validity reactively as values change', () => {
    const form = createFormState<Values>(schema, { name: '', email: '' });
    expect(form.isValid).toBe(false);
    form.values.name = 'Bob';
    form.values.email = 'bob@example.com';
    flushSync();
    expect(form.isValid).toBe(true);
  });

  it('reveals all errors after a submit attempt and skips onValid when invalid', async () => {
    const form = createFormState<Values>(schema, { name: '', email: 'nope' });
    let called = false;
    const onSubmit = form.handleSubmit(() => {
      called = true;
    });

    const event = new Event('submit') as SubmitEvent;
    Object.defineProperty(event, 'preventDefault', { value: () => {} });
    await onSubmit(event);

    flushSync();
    expect(called).toBe(false);
    expect(form.fieldError('name')).toBe('Name is required');
    expect(form.fieldError('email')).toBe('Invalid email');
  });

  it('runs onValid with current values when the form is valid', async () => {
    const form = createFormState<Values>(schema, {
      name: 'Bob',
      email: 'bob@example.com'
    });
    let received: Values | undefined;
    const onSubmit = form.handleSubmit((values) => {
      received = values;
    });

    const event = new Event('submit') as SubmitEvent;
    Object.defineProperty(event, 'preventDefault', { value: () => {} });
    await onSubmit(event);

    expect(received).toEqual({ name: 'Bob', email: 'bob@example.com' });
  });

  it('reset returns values, touched state, and submit flag to defaults', async () => {
    const form = createFormState<Values>(schema, { name: '', email: '' });
    form.values.name = 'changed';
    form.touch('name');
    flushSync();
    expect(form.fieldError('name')).toBeUndefined(); // 'changed' is valid

    form.reset();
    flushSync();
    expect(form.values.name).toBe('');
    expect(form.fieldError('name')).toBeUndefined(); // touch state cleared
  });
});
