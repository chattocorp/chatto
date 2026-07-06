/**
 * Reactive form state backed by a Zod schema.
 *
 * Holds mutable values that bind directly to inputs, derives errors
 * from the schema on every change, and gates error visibility behind
 * "user touched this field" + "submit was attempted" so the form
 * doesn't shout at the user before they've typed anything.
 *
 * ```svelte
 * <script lang="ts">
 *   import { createFormState, z } from '$lib/ui/form';
 *
 *   const schema = z.object({
 *     name: z.string().trim().min(1, 'Name is required').max(30),
 *     email: z.string().email('Enter a valid email')
 *   });
 *
 *   const form = createFormState(schema, { name: '', email: '' });
 *
 *   const submit = form.handleSubmit(async (values) => {
 *     await api.create(values);
 *   });
 * </script>
 *
 * <form onsubmit={submit}>
 *   <TextInput
 *     id="name"
 *     label="Name"
 *     bind:value={form.values.name}
 *     onblur={() => form.touch('name')}
 *     error={form.fieldError('name')}
 *   />
 *   <Button type="submit" disabled={!form.isValid}>Save</Button>
 * </form>
 * ```
 */

import { z } from 'zod';
import { validateForm } from './validation';

export interface FormState<T extends Record<string, unknown>> {
  /** Mutable values. Bind directly: `bind:value={form.values.fieldName}`. */
  values: T;
  /** Errors keyed by dotted field path, recomputed on every value change. */
  readonly errors: Record<string, string>;
  /** True iff the schema currently parses without errors. */
  readonly isValid: boolean;
  /**
   * Returns the error for `name` only when the user has interacted with the
   * field (`touch(name)`) or attempted to submit. Use this for the input's
   * `error` prop so the form doesn't show errors on first paint.
   */
  fieldError<K extends keyof T & string>(name: K): string | undefined;
  /** Mark a field as touched so its error becomes visible. Call from onblur. */
  touch<K extends keyof T & string>(name: K): void;
  /** Reset to the initial values (or `next` if provided). Clears touched state and the submit flag. */
  reset(next?: T): void;
  /**
   * Wrap a submit handler. Calls preventDefault, marks submit-attempted (so
   * any error becomes visible), and only invokes `onValid` when the form is
   * currently valid.
   */
  handleSubmit(
    onValid: (values: T) => void | Promise<void>
  ): (e: SubmitEvent) => void | Promise<void>;
}

export function createFormState<T extends Record<string, unknown>>(
  schema: z.ZodObject<z.ZodRawShape>,
  initialValues: T
): FormState<T> {
  const initial = structuredClone(initialValues);
  let values = $state(structuredClone(initial)) as T;
  let touched = $state<Record<string, boolean>>({});
  let submitAttempted = $state(false);

  const errors = $derived(validateForm(schema, values));
  const isValid = $derived(Object.keys(errors).length === 0);

  return {
    get values() {
      return values;
    },
    set values(next: T) {
      values = next;
    },
    get errors() {
      return errors;
    },
    get isValid() {
      return isValid;
    },
    fieldError(name) {
      if (!submitAttempted && !touched[name]) return undefined;
      return errors[name];
    },
    touch(name) {
      touched[name] = true;
    },
    reset(next) {
      values = structuredClone(next ?? initial);
      touched = {};
      submitAttempted = false;
    },
    handleSubmit(onValid) {
      return async (e: SubmitEvent) => {
        e.preventDefault();
        submitAttempted = true;
        if (Object.keys(errors).length > 0) return;
        await onValid(values);
      };
    }
  };
}
