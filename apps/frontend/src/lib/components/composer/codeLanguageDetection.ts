/** Save language guesses only for code blocks changed during this editing session. */
import type { Node } from '@tiptap/pm/model';
import { Plugin, PluginKey, type EditorState } from '@tiptap/pm/state';
import type { EditorView } from '@tiptap/pm/view';
import { detectCodeLanguage } from '$lib/codeHighlighting';

type ChangedBlock = { node: Node; pasted: boolean };
const detectionKey = new PluginKey<Map<number, ChangedBlock>>('codeLanguageDetection');

function selectedCodeBlock(state: EditorState): number | null {
  const { $from } = state.selection;
  for (let depth = $from.depth; depth > 0; depth--) {
    if ($from.node(depth).type.name === 'codeBlock') return $from.before(depth);
  }
  return null;
}

/** Own detection requests with the editor view; stale results never change its document. */
export function createCodeLanguageDetectionPlugin(
  detect: (code: string) => Promise<string | null> = detectCodeLanguage
): Plugin<Map<number, ChangedBlock>> {
  const attempted = new WeakSet<ChangedBlock>();
  let disposed = false;

  function detectChangedBlocks(view: EditorView, blurred = false) {
    const selected = selectedCodeBlock(view.state);
    for (const [position, entry] of detectionKey.getState(view.state) ?? []) {
      const { node, pasted } = entry;
      if ((!blurred && !pasted && selected === position) || attempted.has(entry)) continue;
      attempted.add(entry);
      void detect(node.textContent).then(
        (language) => {
          if (disposed || view.isDestroyed || !language) return;
          // The state retains this entry only while the same code and attributes survive.
          const current = [...(detectionKey.getState(view.state) ?? [])].find(
            ([, candidate]) => candidate === entry
          );
          if (!current) return;
          view.dispatch(
            view.state.tr.setNodeMarkup(current[0], undefined, { ...node.attrs, language })
          );
        },
        // Detection is optional. A failed load leaves the user's code unchanged.
        () => {}
      );
    }
  }

  return new Plugin({
    key: detectionKey,
    state: {
      init: () => new Map(),
      apply(transaction, previous, oldState, newState) {
        // Loading a draft or existing message is not an authoring action.
        if (transaction.getMeta('preventUpdate')) return new Map();
        if (!transaction.docChanged) return previous;
        const changed = new Map<number, ChangedBlock>();
        for (const [position, entry] of previous) {
          // Map inside the node: decoration refreshes replace its opening token.
          const mapped = transaction.mapping.mapResult(position + 1);
          if (mapped.deleted) continue;
          const $mapped = newState.doc.resolve(mapped.pos);
          if (!$mapped.depth) continue;
          const mappedPosition = $mapped.before();
          const node = $mapped.parent;
          if (node?.type.name === 'codeBlock' && !node.attrs.language && node.textContent.trim()) {
            changed.set(mappedPosition, node.eq(entry.node) ? entry : { node, pasted: false });
          }
        }
        const position = selectedCodeBlock(newState);
        const oldPosition = selectedCodeBlock(oldState);
        const node = position === null ? null : newState.doc.nodeAt(position);
        const oldNode = oldPosition === null ? null : oldState.doc.nodeAt(oldPosition);
        if (
          position !== null &&
          node &&
          !node.attrs.language &&
          node.textContent.trim() &&
          node.textContent !== oldNode?.textContent
        ) {
          changed.set(position, { node, pasted: transaction.getMeta('uiEvent') === 'paste' });
        }
        return changed;
      }
    },
    props: {
      handleDOMEvents: {
        blur(view) {
          detectChangedBlocks(view, true);
          return false;
        }
      }
    },
    view: () => ({
      update: (view) => detectChangedBlocks(view),
      destroy: () => {
        disposed = true;
      }
    })
  });
}
