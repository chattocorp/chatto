/** Style parsed double emphasis by its delimiter without matching text in code. */
import { syntaxTree } from '@codemirror/language';
import type { Range } from '@codemirror/state';
import { Decoration, ViewPlugin, type EditorView, type ViewUpdate } from '@codemirror/view';
import { styleTags, tags } from '@lezer/highlight';
import type { MarkdownConfig } from '@lezer/markdown';

/** The decorations below distinguish underline from bold after Markdown parsing. */
export const sourceEmphasisSyntax: MarkdownConfig = {
  props: [styleTags({ '*/StrongEmphasis/...': tags.content })]
};

function emphasisDecorations(view: EditorView) {
  const ranges: Range<Decoration>[] = [];
  syntaxTree(view.state).iterate({
    from: view.viewport.from,
    to: view.viewport.to,
    enter(node) {
      if (node.name !== 'StrongEmphasis') return;
      const underline = view.state.sliceDoc(node.from, node.from + 2) === '__';
      ranges.push(
        Decoration.mark({ class: underline ? 'cm-format-underline' : 'cm-format-strong' }).range(
          node.from,
          node.to
        )
      );
    }
  });
  return Decoration.set(ranges, true);
}

/** Keep visible emphasis current after edits, scrolling, and background parsing. */
export const sourceEmphasisHighlighting = ViewPlugin.fromClass(
  class {
    decorations;

    constructor(view: EditorView) {
      this.decorations = emphasisDecorations(view);
    }

    update(update: ViewUpdate) {
      if (
        update.docChanged ||
        update.viewportChanged ||
        syntaxTree(update.startState) !== syntaxTree(update.state)
      ) {
        this.decorations = emphasisDecorations(update.view);
      }
    }
  },
  { decorations: (plugin) => plugin.decorations }
);
