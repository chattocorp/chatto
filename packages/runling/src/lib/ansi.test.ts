import { expect, test } from "vitest";
import { ansiTokens } from "./ansi.ts";

test("preserves plain text, whitespace, and HTML as literal text", () => {
  const text = "<img src=x onerror=alert(1)> & hello\n  world";
  expect(ansiTokens(text)).toEqual([{ text, style: "" }]);
});

test("renders foreground, background, and resets", () => {
  expect(ansiTokens("\x1b[31;44mred\x1b[0m normal")).toEqual([
    { text: "red", style: "color:#bb0000;background-color:#0000bb" },
    { text: " normal", style: "" },
  ]);
});

test("supports RGB, 256 colors, and combined decorations", () => {
  const [token] = ansiTokens("\x1b[38;2;12;34;56;48;5;196;1;3;4;9mhello");
  expect(token?.style).toContain("color:#0c2238");
  expect(token?.style).toContain("background-color:#ff0000");
  expect(token?.style).toContain("font-weight:700");
  expect(token?.style).toContain("font-style:italic");
  expect(token?.style).toContain("text-decoration-line:underline line-through");
});

test("reparsing live output does not leak styles between renders", () => {
  ansiTokens("\x1b[31mred");
  expect(ansiTokens("plain")).toEqual([{ text: "plain", style: "" }]);
  expect(ansiTokens("\x1b[31mred\x1b[0m plain").at(-1)?.style).toBe("");
});
