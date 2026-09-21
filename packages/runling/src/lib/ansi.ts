import { createColorPalette, parseAnsiSequences } from "ansi-sequence-parser";

const palette = createColorPalette();

/** Convert terminal styles into CSS; text remains text, never HTML. */
export function ansiTokens(text: string) {
  return parseAnsiSequences(text).map((token) => {
    const styles: string[] = [];
    let foreground = token.foreground
      ? palette.value(token.foreground)
      : undefined;
    let background = token.background
      ? palette.value(token.background)
      : undefined;
    if (token.decorations.has("reverse"))
      [foreground, background] = [
        background ?? "#ffffff",
        foreground ?? "#24324b",
      ];
    if (foreground) styles.push(`color:${foreground}`);
    if (background) styles.push(`background-color:${background}`);
    if (token.decorations.has("bold")) styles.push("font-weight:700");
    if (token.decorations.has("dim")) styles.push("opacity:0.6");
    if (token.decorations.has("italic")) styles.push("font-style:italic");
    if (token.decorations.has("hidden")) styles.push("visibility:hidden");
    const lines = [];
    if (token.decorations.has("underline")) lines.push("underline");
    if (token.decorations.has("strikethrough")) lines.push("line-through");
    if (token.decorations.has("overline")) lines.push("overline");
    if (lines.length) styles.push(`text-decoration-line:${lines.join(" ")}`);
    return { text: token.value, style: styles.join(";") };
  });
}
