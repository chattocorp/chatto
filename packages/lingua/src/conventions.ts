import type { HtmlTranslationKey, LocalizedHtml } from "./types.js";

const countSuffixes = [
  "_count",
  ".count",
  "_count_html",
  ".count_html",
  ".count.html",
];

export function isCountTranslationKey(key: string): boolean {
  return countSuffixes.some((suffix) => key.endsWith(suffix));
}

export function isHtmlTranslationKey(key: string): key is HtmlTranslationKey {
  return key.endsWith("_html") || key.endsWith(".html");
}

export function asLocalizedHtml(value: string): LocalizedHtml {
  return value as LocalizedHtml;
}
