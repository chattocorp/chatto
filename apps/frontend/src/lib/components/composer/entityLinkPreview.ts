/** ProseMirror decorations preview automatic web URLs without changing the editable document. */
import { Plugin, PluginKey, TextSelection } from '@tiptap/pm/state';
import { Decoration, DecorationSet } from '@tiptap/pm/view';
import { mount, unmount } from 'svelte';
import EntityLink from '../messages/EntityLink.svelte';
import { parseWebLink } from '$lib/navigation/webLink';

/** Widgets own their Svelte previews; ProseMirror owns URL text, selection, and clipboard data. */
export function entityLinkPreview(context: Map<unknown, unknown>): Plugin {
  const key = new PluginKey('entityLinkPreview');
  let focused = false;
  return new Plugin({
    key,
    props: {
      handleDOMEvents: {
        focus(view) {
          focused = true;
          view.dispatch(view.state.tr.setMeta(key, true));
          return false;
        },
        blur(view) {
          focused = false;
          view.dispatch(view.state.tr.setMeta(key, false));
          return false;
        }
      },
      decorations(state) {
        const decorations: Decoration[] = [];
        state.doc.descendants((node, from) => {
          if (!node.isText) return;
          const link = node.marks.find((mark) => mark.type.name === 'link');
          if (!link || link.attrs.explicitLabel || link.attrs.title) return;
          const href = link.attrs.href as string;
          if (node.text !== href && `http://${node.text}` !== href) return;
          if (!parseWebLink(href)) return;
          const to = from + node.nodeSize;
          // A caret beside the URL (including just after paste) does not edit its text.
          if (focused && state.selection.from < to && state.selection.to > from) return;

          decorations.push(Decoration.inline(from, to, { class: 'composer-entity-source' }));
          // Give the browser a visible DOM boundary after hidden URL text, so a
          // paste leaves the caret after the pill instead of before its widget.
          decorations.push(
            Decoration.widget(
              to,
              () => {
                const boundary = document.createElement('span');
                boundary.textContent = '\u200b';
                boundary.setAttribute('aria-hidden', 'true');
                return boundary;
              },
              { key: `${to}:entity-boundary`, side: -1, marks: [] }
            )
          );
          let dispose: (() => void) | undefined;
          decorations.push(
            Decoration.widget(
              from,
              (view, getPos) => {
                const preview = document.createElement('span');
                preview.className = 'composer-entity-preview';
                const component = mount(EntityLink, {
                  target: preview,
                  context,
                  props: { href }
                });
                preview.addEventListener(
                  'click',
                  (event) => {
                    event.preventDefault();
                    event.stopImmediatePropagation();
                    const position = getPos();
                    if (position === undefined) return;
                    focused = true;
                    view.dispatch(
                      view.state.tr.setSelection(TextSelection.create(view.state.doc, position + 1))
                    );
                    view.focus();
                  },
                  true
                );
                dispose = () => {
                  void unmount(component);
                };
                return preview;
              },
              {
                key: `${from}:${href}`,
                side: -1,
                marks: [],
                stopEvent: () => true,
                destroy: () => dispose?.()
              }
            )
          );
        });
        return DecorationSet.create(state.doc, decorations);
      }
    }
  });
}
