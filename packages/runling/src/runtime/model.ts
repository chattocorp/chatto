export interface ModelReference {
  provider: string;
  id: string;
}

export function parseModelReference(reference: string): ModelReference {
  const separator = reference.indexOf("/");

  if (separator <= 0 || separator === reference.length - 1) {
    throw new Error(
      `Invalid model reference "${reference}"; expected "provider/model-id"`,
    );
  }

  return {
    provider: reference.slice(0, separator),
    id: reference.slice(separator + 1),
  };
}
