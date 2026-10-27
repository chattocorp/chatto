import { Plugin } from '@tiptap/pm/state';
import { Decoration, DecorationSet } from '@tiptap/pm/view';
import type { Node } from '@tiptap/pm/model';

/** Editable code keeps its text model intact; numbers are non-editable decorations. */
export function codeBlockLineNumbers(): Plugin {
  function decorations(doc: Node): DecorationSet {
    const result: Decoration[] = [];
    doc.descendants((node, position) => {
      if (node.type.name !== 'codeBlock') return;
      const lines = node.textContent.split('\n');
      const digits = String(lines.length).length;
      let offset = 0;
      lines.forEach((line, index) => {
        result.push(
          Decoration.widget(
            position + 1 + offset,
            () => {
              const number = document.createElement('span');
              number.className = 'code-line-number';
              number.dataset.line = String(index + 1);
              number.style.setProperty('--code-line-digits', String(digits));
              number.setAttribute('aria-hidden', 'true');
              return number;
            },
            { side: -1, ignoreSelection: true }
          )
        );
        offset += line.length + 1;
      });
      return false;
    });
    return DecorationSet.create(doc, result);
  }
  return new Plugin<DecorationSet>({
    state: {
      init: (_, state) => decorations(state.doc),
      apply: (transaction, previous) =>
        transaction.docChanged ? decorations(transaction.doc) : previous
    },
    props: {
      decorations(state) {
        return this.getState(state);
      }
    }
  });
}
