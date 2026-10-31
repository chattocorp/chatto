import type { NodeViewRenderer } from '@tiptap/core';
import { normalizeCodeLanguage } from '$lib/codeHighlighting';
import { attachCodeFades } from '$lib/ui/codeScrollFades';

/** ProseMirror owns only `code`; the node view owns the scroll viewport and fades. */
export const codeBlockView: NodeViewRenderer = ({ node }) => {
  const dom = document.createElement('pre');
  const body = document.createElement('div');
  body.className = 'composer-code-body';
  const viewport = document.createElement('div');
  viewport.className = 'composer-code-viewport';
  const code = document.createElement('code');
  viewport.append(code);
  body.append(viewport);
  dom.append(body);
  const updateLanguage = (language: string | null) => {
    const normalized = normalizeCodeLanguage(language);
    if (dom.dataset.language === normalized) return;
    dom.dataset.language = normalized;
    code.className = `language-${normalized}`;
  };
  updateLanguage(node.attrs.language);
  const cleanup = attachCodeFades(code, body);
  return {
    dom,
    contentDOM: code,
    update(next) {
      if (next.type !== node.type) return false;
      updateLanguage(next.attrs.language);
      return true;
    },
    // Fade visibility changes must not become editor transactions.
    ignoreMutation: (mutation) => mutation.type !== 'selection' && !code.contains(mutation.target),
    destroy: cleanup
  };
};
