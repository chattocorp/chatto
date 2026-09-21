export type ConcatPart = string | readonly ConcatPart[];

export function concat(...parts: ConcatPart[]): string {
  const lines: string[] = [];

  const append = (part: ConcatPart) => {
    if (typeof part === "string") {
      lines.push(part);
      return;
    }

    for (const nestedPart of part) {
      append(nestedPart);
    }
  };

  for (const part of parts) {
    append(part);
  }

  return lines.join("\n");
}
