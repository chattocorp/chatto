import colors from 'color-name';

/** Shared terminal color policy for server diagnostics and workflow output. */
export function terminalColors(stream: { isTTY?: boolean }): boolean {
  return (
    process.env.NO_COLOR === undefined &&
    process.env.FORCE_COLOR !== '0' &&
    (process.env.FORCE_COLOR !== undefined || Boolean(stream.isTTY))
  );
}

export function ansiColor(color: string): string {
  const rgb =
    color.startsWith('#') && /^#[0-9a-f]{6}$/i.test(color)
      ? [1, 3, 5].map((offset) => Number.parseInt(color.slice(offset, offset + 2), 16))
      : Object.hasOwn(colors, color.toLowerCase())
        ? colors[color.toLowerCase() as keyof typeof colors]
        : undefined;
  return rgb ? `\x1b[38;2;${rgb.join(';')}m` : '';
}
