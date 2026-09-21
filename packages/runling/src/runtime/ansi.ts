import colors from "color-name";

export function ansiColor(color: string): string {
  const rgb =
    color.startsWith("#") && /^#[0-9a-f]{6}$/i.test(color)
      ? [1, 3, 5].map((offset) =>
          Number.parseInt(color.slice(offset, offset + 2), 16),
        )
      : Object.hasOwn(colors, color.toLowerCase())
        ? colors[color.toLowerCase() as keyof typeof colors]
        : undefined;
  return rgb ? `\x1b[38;2;${rgb.join(";")}m` : "";
}
