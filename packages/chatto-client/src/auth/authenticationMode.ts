// The bundled same-origin browser uses only its HttpOnly session cookie.
// Programmatic clients omit this header and keep the bearer-token response.
export const browserCookieAuthenticationHeaders = {
  'X-Chatto-Authentication-Mode': 'cookie'
} as const;
