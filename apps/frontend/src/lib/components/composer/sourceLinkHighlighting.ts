/** Apply link styling only to the label of a complete inline Markdown link. */
import { syntaxTree } from '@codemirror/language';
import { Decoration, EditorView, ViewPlugin, type ViewUpdate } from '@codemirror/view';
import type { Range } from '@codemirror/state';
import { styleTags, tags } from '@lezer/highlight';
import type { MarkdownConfig } from '@lezer/markdown';

/** Keep link source neutral while preserving image and automatic URL highlighting. */
export const sourceLinkSyntax: MarkdownConfig = {
  // A parent context makes this rule more specific than Lezer's default Link rule.
  props: [styleTags({ '*/Link/...': tags.content, 'Link/URL': tags.content })]
};

const labelMark = Decoration.mark({ class: 'cm-link-label' });

function linkLabels(view: EditorView) {
  const ranges: Range<Decoration>[] = [];
  syntaxTree(view.state).iterate({
    from: view.viewport.from,
    to: view.viewport.to,
    enter(node) {
      if (node.name !== 'Link') return;
      const marks = node.node.getChildren('LinkMark');
      // Lezer also emits Link nodes for bare brackets and unfinished links.
      // Only the four marks of [label](destination) confirm an inline link.
      if (marks.length !== 4) return;
      const start = marks[0]!.to;
      const end = marks[1]!.from;
      if (start < end) ranges.push(labelMark.range(start, end));
    }
  });
  return Decoration.set(ranges, true);
}

/** Recompute visible labels when edits, scrolling, or background parsing change syntax. */
export const sourceLinkHighlighting = ViewPlugin.fromClass(
  class {
    decorations;

    constructor(view: EditorView) {
      this.decorations = linkLabels(view);
    }

    update(update: ViewUpdate) {
      if (
        update.docChanged ||
        update.viewportChanged ||
        syntaxTree(update.startState) !== syntaxTree(update.state)
      ) {
        this.decorations = linkLabels(update.view);
      }
    }
  },
  { decorations: (plugin) => plugin.decorations }
);
