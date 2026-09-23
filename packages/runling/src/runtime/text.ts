export function toSingleLine(text: string) {
  return text.replace(/\s+/g, " ").trim();
}

export function containsMalformedToolCall(text: string) {
  return /functions\.[\w-]+:\d+\s*\{/.test(text);
}
