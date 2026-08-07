/** Validate the renderer-provided destination before native navigation. */
export function oauthNavigationUrl(target: unknown): string {
  if (typeof target !== "string") throw new TypeError("Invalid sign-in URL.");

  const url = new URL(target);
  if (url.protocol !== "https:" && url.protocol !== "http:") {
    throw new TypeError("Sign-in URLs must use HTTP or HTTPS.");
  }
  if (url.username || url.password) {
    throw new TypeError("Sign-in URLs must not contain credentials.");
  }
  return url.href;
}
