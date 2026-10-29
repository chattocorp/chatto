import { Terminal, type IBufferCell } from '@xterm/headless';

/** Static ANSI screens. No terminal input, external I/O, or persistent terminal state. */
const COLUMNS = 80;
const ROWS = 100;
/** Defensive parser limit, shared with the Markdown source fallback. */
export const MAX_SOURCE_LENGTH = 10_000;
const PALETTE = [
  '#000000',
  '#aa0000',
  '#00aa00',
  '#aa5500',
  '#0000aa',
  '#aa00aa',
  '#00aaaa',
  '#aaaaaa',
  '#555555',
  '#ff5555',
  '#55ff55',
  '#ffff55',
  '#5555ff',
  '#ff55ff',
  '#55ffff',
  '#ffffff'
];

/** Convert only numeric terminal colors to CSS; source text never becomes CSS. */
function color(value: number, rgb: boolean): string {
  if (rgb) return `#${value.toString(16).padStart(6, '0')}`;
  if (value < 16) return PALETTE[value];
  if (value >= 232) {
    const grey = 8 + (value - 232) * 10;
    return `rgb(${grey},${grey},${grey})`;
  }
  const cube = value - 16;
  const level = (n: number) => (n === 0 ? 0 : 55 + n * 40);
  return `rgb(${level(Math.floor(cube / 36))},${level(Math.floor(cube / 6) % 6)},${level(cube % 6)})`;
}

function cellStyle(cell: IBufferCell): string {
  let fg = cell.isFgDefault()
    ? '#aaaaaa'
    : color(
        cell.getFgColor() + (cell.isBold() && cell.isFgPalette() && cell.getFgColor() < 8 ? 8 : 0),
        cell.isFgRGB()
      );
  let bg = cell.isBgDefault() ? '#000000' : color(cell.getBgColor(), cell.isBgRGB());
  if (cell.isInverse()) [fg, bg] = [bg, fg];
  if (cell.isInvisible()) fg = bg;
  // SGR 2 dims only the text. Span opacity would also fade painted backgrounds.
  if (cell.isDim()) fg = `color-mix(in srgb, ${fg} 50%, ${bg})`;
  const decorations = [
    cell.isUnderline() ? 'underline' : '',
    cell.isStrikethrough() ? 'line-through' : '',
    cell.isOverline() ? 'overline' : ''
  ]
    .filter(Boolean)
    .join(' ');
  return (
    `color:${fg};background-color:${bg}` +
    (cell.isBold() ? ';font-weight:bold' : '') +
    (cell.isItalic() ? ';font-style:italic' : '') +
    (decorations ? `;text-decoration:${decorations}` : '')
  );
}

function escapeText(value: string): string {
  return value.replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;');
}

/**
 * Parse one Unicode message fence and return escaped, styled screen cells.
 * The final 80 × 100 screen is rendered without scrollback or blinking.
 * Terminal commands cannot resize the screen or access the browser. Each call
 * owns and disposes its terminal, including when parsing fails.
 */
export async function renderAnsiArt(source: string): Promise<string> {
  if (source.length > MAX_SOURCE_LENGTH) throw new Error('ANSI source exceeds the message limit');
  const terminal = new Terminal({
    cols: COLUMNS,
    rows: ROWS,
    scrollback: 0,
    convertEol: true,
    allowProposedApi: true,
    logLevel: 'off'
  });
  try {
    await new Promise<void>((resolve) => terminal.write(source, resolve));
    const buffer = terminal.buffer.active;
    const lines: string[] = [];
    let lastContentRow = 0;
    for (let y = 0; y < ROWS; y++) {
      const line = buffer.getLine(y);
      let html = '';
      let text = '';
      let style = '';
      const flush = () => {
        if (text) html += `<span style="${style}">${escapeText(text)}</span>`;
        text = '';
      };
      for (let x = 0; x < COLUMNS; x++) {
        const cell = line?.getCell(x);
        if (!cell || cell.getWidth() === 0) continue;
        const nextStyle = cellStyle(cell);
        if (nextStyle !== style) {
          flush();
          style = nextStyle;
        }
        text += cell.getChars() || ' ';
        // Keep painted spaces, but remove unused rows below the artwork.
        if (cell.getChars().trim() || !cell.isBgDefault() || cell.isInverse()) lastContentRow = y;
      }
      flush();
      lines.push(html);
    }
    return lines.slice(0, lastContentRow + 1).join('\n');
  } finally {
    terminal.dispose();
  }
}
