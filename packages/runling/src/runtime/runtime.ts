export type JsonValue =
  null | boolean | number | string | JsonValue[] | { [key: string]: JsonValue };

/** Check values before they cross a JSON output or event boundary. */
export function isJsonValue(value: unknown, ancestors = new Set<object>()): value is JsonValue {
  if (value === null || typeof value === 'string' || typeof value === 'boolean') return true;
  if (typeof value === 'number') return Number.isFinite(value);
  if (typeof value !== 'object' || ancestors.has(value)) return false;

  const prototype = Object.getPrototypeOf(value);
  if (!Array.isArray(value) && prototype !== Object.prototype && prototype !== null) return false;

  ancestors.add(value);
  // JSON omits undefined object properties. Array entries must be valid values.
  const valid = Object.values(value).every(
    (entry) => (entry === undefined && !Array.isArray(value)) || isJsonValue(entry, ancestors)
  );
  ancestors.delete(value);
  return valid;
}

/** Check that a value can be published as a JSON object. */
export function isJsonObject(value: unknown): value is { [key: string]: JsonValue } {
  return value !== null && typeof value === 'object' && !Array.isArray(value) && isJsonValue(value);
}

export interface WorkflowResult {
  summary: string;
  /** Human-readable Markdown shown after the summary in interactive mode. */
  details?: string;
  outputs?: Record<string, JsonValue>;
}

export type WorkflowReturn = WorkflowResult | JsonValue | undefined | void;
