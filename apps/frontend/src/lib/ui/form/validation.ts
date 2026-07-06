/**
 * Zod-based validation for forms.
 *
 * Use Zod schemas directly for validation. The `validate` helper
 * extracts the first error message for simple field-level validation.
 */

import { z } from 'zod';

export { z };

/**
 * Validate a value against a Zod schema.
 * Returns the first error message, or undefined if valid.
 */
export function validate<T>(schema: z.ZodType<T>, value: unknown): string | undefined {
  const result = schema.safeParse(value);
  if (result.success) return undefined;
  return result.error.issues[0]?.message;
}

/**
 * Validate an object against a Zod object schema.
 * Returns a map of field names to error messages.
 */
export function validateForm<T extends z.ZodRawShape>(
  schema: z.ZodObject<T>,
  data: unknown
): Record<string, string> {
  const result = schema.safeParse(data);
  if (result.success) return {};

  const errors: Record<string, string> = {};
  for (const issue of result.error.issues) {
    const path = issue.path.join('.');
    if (path && !errors[path]) {
      errors[path] = issue.message;
    }
  }
  return errors;
}
