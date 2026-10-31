/** ProseMirror decorations preview automatic web URLs without changing the editable document. */
import { Plugin, PluginKey, TextSelection } from '@tiptap/pm/state';
import { Decoration, DecorationSet } from '@tiptap/pm/view';
import { mount, unmount } from 'svelte';
import type { Node } from '@tiptap/pm/model';
import EntityLink from '../messages/EntityLink.svelte';
import { parseWebLink } from '$lib/navigation/webLink';

/** Only automatic URL text can be replaced by a preview. */
function previewHref(node: Node): string | undefined {
  if (!node.isText) return;
  const link = node.marks.find((mark) => mark.type.name === 'link');
  if (!link || link.attrs.explicitLabel || link.attrs.title) return;
  const href = link.attrs.href as string;
  if (node.text !== href && `http://${node.text}` !== href) return;
  if (parseWebLink(href)) return href;
}

/** Widgets own their Svelte previews; ProseMirror owns URL text, selection, and clipboard data. */
export function entityLinkPreview(context: Map<unknown, unknown>): Plugin {
  const key = new PluginKey('entityLinkPreview');
  let focused = false;
  return new Plugin({
    key,
    props: {
      handleKeyDown(view, event) {
        if (event.altKey || event.ctrlKey || event.metaKey) return false;
        if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight') return false;
        const { selection } = view.state;
        if (!(selection instanceof TextSelection) || !selection.empty) return false;
        const direction = event.key === 'ArrowLeft' ? -1 : 1;
        const { $head } = selection;
        // These lookups return the whole text node, also when the caret is inside it.
        const { node } =
          direction < 0
            ? $head.parent.childBefore($head.parentOffset)
            : $head.parent.childAfter($head.parentOffset);
        if (!node || !previewHref(node)) return false;
        if ($head.textOffset && $head.textOffset !== (direction < 0 ? 1 : node.nodeSize - 1)) {
          return false;
        }

        // Native caret movement skips display:none URL text and can leave the
        // caret in a widget boundary. Enter the document text explicitly so
        // decorations reveal the URL before the next edit. Also handle leaving
        // the first/last character: collapsing the text changes the DOM caret.
        const head = selection.head + direction;
        view.dispatch(
          view.state.tr.setSelection(
            TextSelection.create(view.state.doc, event.shiftKey ? selection.anchor : head, head)
          )
        );
        return true;
      },
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
          const href = previewHref(node);
          if (!href) return;
          const to = from + node.nodeSize;
          // A caret beside the URL (including just after paste) does not edit its text.
          if (focused && state.selection.from < to && state.selection.to > from) return;

          decorations.push(Decoration.inline(from, to, { class: 'composer-entity-source' }));
          // Give the browser a visible caret boundary on both sides of hidden
          // URL text, including when the pill is the only paragraph content.
          for (const position of [from, to])
            decorations.push(
              Decoration.widget(
                position,
                () => {
                  const boundary = document.createElement('span');
                  boundary.textContent = '\u200b';
                  boundary.setAttribute('aria-hidden', 'true');
                  return boundary;
                },
                { key: `${position}:entity-boundary`, side: -1, marks: [] }
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
                // The URL's start position belongs before its preview. When
                // text collapses back to a pill, keep the caret on that side.
                side: 1,
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
