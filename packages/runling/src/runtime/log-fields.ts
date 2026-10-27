/** Structured application metadata. Callers must exclude private content and credentials. */
export type LogFields = Readonly<Record<string, string | number | boolean | null | undefined>>;

/** Stable text form shared by terminals and the console; JSON escaping prevents line injection. */
export function formatLogFields(fields?: LogFields): string {
  const values = Object.entries(fields ?? {}).filter(([, value]) => value !== undefined);
  return values.length
    ? ` ${values.map(([key, value]) => `${JSON.stringify(key).slice(1, -1)}=${JSON.stringify(value)}`).join(' ')}`
    : '';
}
