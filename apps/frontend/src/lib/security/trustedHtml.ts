const MARKDOWN_POLICY_NAME = 'chatto-markdown-html';
const POLICY_CACHE_KEY = '__chattoMarkdownTrustedTypesPolicy';

type TrustedHTMLValue = string | { toString(): string };

type TrustedTypePolicy = {
  createHTML(html: string): TrustedHTMLValue;
};

type TrustedTypesGlobal = typeof globalThis & {
  trustedTypes?: {
    createPolicy(
      name: string,
      policy: {
        createHTML(html: string): string;
      }
    ): TrustedTypePolicy;
  };
  [POLICY_CACHE_KEY]?: TrustedTypePolicy;
};

const trustedTypesGlobal = globalThis as TrustedTypesGlobal;

function getMarkdownPolicy(): TrustedTypePolicy | null {
  if (!trustedTypesGlobal.trustedTypes) return null;

  trustedTypesGlobal[POLICY_CACHE_KEY] ??= trustedTypesGlobal.trustedTypes.createPolicy(
    MARKDOWN_POLICY_NAME,
    {
      createHTML(html) {
        return html;
      }
    }
  );
  return trustedTypesGlobal[POLICY_CACHE_KEY];
}

/**
 * Trusts HTML produced by Chatto's markdown renderer and its reviewed
 * post-processing steps. Do not use this for raw user-authored HTML.
 */
export function trustedMarkdownHtml(html: string): TrustedHTMLValue {
  return getMarkdownPolicy()?.createHTML(html) ?? html;
}

/**
 * DOMParser is also a Trusted Types sink in Chromium. The cast is compile-time
 * only; browsers receive the TrustedHTML object when Trusted Types are present.
 */
export function parseTrustedMarkdownHtml(html: string): Document {
  return new DOMParser().parseFromString(trustedMarkdownHtml(html) as string, 'text/html');
}
