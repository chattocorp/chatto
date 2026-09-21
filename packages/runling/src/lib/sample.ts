/** A starting point for the JSON editor; the server still validates the request. */
export function sample(value: unknown, depth = 0): unknown {
  if (typeof value !== "object" || value === null || Array.isArray(value)) return null;
  const schema = value as Record<string, unknown>;
  if (depth > 5) return null;
  if ("default" in schema) return schema.default;
  if ("const" in schema) return schema.const;
  if (Array.isArray(schema.examples) && schema.examples.length)
    return schema.examples[0];
  if (Array.isArray(schema.enum)) return schema.enum[0];
  const branches = schema.anyOf ?? schema.oneOf;
  if (Array.isArray(branches) && branches[0])
    return sample(branches[0], depth + 1);
  if (schema.type === "object" || schema.properties) {
    return Object.fromEntries(
      Object.entries(
        (schema.properties ?? {}) as Record<string, Record<string, unknown>>,
      ).map(([key, value]) => [key, sample(value, depth + 1)]),
    );
  }
  if (schema.type === "array")
    return Array.from(
      { length: Math.min(5, Number(schema.minItems) || 0) },
      () => sample((schema.items ?? {}) as Record<string, unknown>, depth + 1),
    );
  if (schema.type === "boolean") return false;
  if (schema.type === "number" || schema.type === "integer")
    return schema.minimum ?? 0;
  if (schema.type === "null") return null;
  return "example";
}
