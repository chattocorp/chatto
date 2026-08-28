import { InterpolationError } from "./errors.js";
import type { InterpolationValues } from "./types.js";

const placeholderPattern = /\{([A-Za-z_][A-Za-z0-9_.-]*)\}/g;

export function translationPlaceholders(template: string): readonly string[] {
  return [...template.matchAll(placeholderPattern)].map(
    (match) => match[1] as string,
  );
}

function escapeHtml(value: string): string {
  return value
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

export function formatTranslation(
  template: string,
  values: InterpolationValues | undefined,
  escapeInterpolation: boolean,
): string {
  return template.replace(placeholderPattern, (_placeholder, name: string) => {
    const value = values?.[name];
    if (value === undefined) {
      throw new InterpolationError(`Missing interpolation value "${name}"`);
    }
    const formatted = String(value);
    return escapeInterpolation ? escapeHtml(formatted) : formatted;
  });
}
