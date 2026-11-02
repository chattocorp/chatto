/** Show local Chatto link pills while CodeMirror retains the complete Markdown source. */
import { syntaxTree } from '@codemirror/language';
import { EditorSelection, Prec, type Range } from '@codemirror/state';
import {
  Decoration,
  EditorView,
  ViewPlugin,
  WidgetType,
  keymap,
  type ViewUpdate
} from '@codemirror/view';
import { mount, unmount } from 'svelte';
import EntityLink from '../messages/EntityLink.svelte';
import { hasEntityLinkData, parseEntityLink, parseMessageLink } from '$lib/chatLinks';

// CodeMirror can reuse DOM with a new WidgetType instance. Keep cleanup with the DOM.
const previewDisposers = new WeakMap<HTMLElement, () => void>();

/** Each rendered widget owns a Svelte component until CodeMirror removes its DOM. */
class EntityPreview extends WidgetType {
  constructor(
    readonly href: string,
    readonly context: Map<unknown, unknown>
  ) {
    super();
  }

  eq(other: EntityPreview) {
    return this.href === other.href;
  }

  toDOM(view: EditorView) {
    const preview = document.createElement('span');
    preview.className = 'composer-entity-preview';
    const component = mount(EntityLink, {
      target: preview,
      context: this.context,
      props: { href: this.href }
    });
    previewDisposers.set(preview, () => void unmount(component));
    preview.addEventListener(
      'click',
      (event) => {
        event.preventDefault();
        event.stopImmediatePropagation();
        const from = view.posAtDOM(preview);
        view.dispatch({ selection: EditorSelection.cursor(from + 1), scrollIntoView: true });
        view.focus();
      },
      true
    );
    return preview;
  }

  destroy(dom: HTMLElement) {
    previewDisposers.get(dom)?.();
    previewDisposers.delete(dom);
  }
}

/** Preview automatic links only. Explicit labels, images, and code stay visible. */
function previewLinks(view: EditorView) {
  const links: { from: number; to: number; href: string }[] = [];
  syntaxTree(view.state).iterate({
    from: view.viewport.from,
    to: view.viewport.to,
    enter(node) {
      if (
        [
          'Link',
          'LinkReference',
          'Image',
          'InlineCode',
          'FencedCode',
          'CodeBlock',
          'HTMLBlock'
        ].includes(node.name)
      )
        return false;
      if (node.name !== 'URL') return;
      const href = view.state.doc.sliceString(node.from, node.to);
      const target = parseEntityLink(href);
      if (!(target ? hasEntityLinkData(target) : parseMessageLink(href)?.serverId)) return;
      const range = node.node.parent?.name === 'Autolink' ? node.node.parent : node;
      links.push({ from: range.from, to: range.to, href });
    }
  });
  return links;
}

/** Reveal source on click or caret entry; replacements never change clipboard or message text. */
export function sourceEntityLinkPreview(context: Map<unknown, unknown>) {
  function decorations(view: EditorView) {
    const ranges: Range<Decoration>[] = [];
    for (const { from, to, href } of previewLinks(view)) {
      if (
        view.hasFocus &&
        view.state.selection.ranges.some((range) => range.from < to && range.to > from)
      )
        continue;
      ranges.push(Decoration.replace({ widget: new EntityPreview(href, context) }).range(from, to));
    }
    return Decoration.set(ranges, true);
  }

  return [
    ViewPlugin.fromClass(
      class {
        decorations;

        constructor(view: EditorView) {
          this.decorations = decorations(view);
        }

        update(update: ViewUpdate) {
          if (
            update.docChanged ||
            update.selectionSet ||
            update.focusChanged ||
            update.viewportChanged ||
            syntaxTree(update.startState) !== syntaxTree(update.state)
          )
            this.decorations = decorations(update.view);
        }
      },
      { decorations: (plugin) => plugin.decorations }
    ),
    Prec.high(
      keymap.of(
        ([-1, 1] as const).map((direction) => ({
          key: direction < 0 ? 'ArrowLeft' : 'ArrowRight',
          run(view: EditorView) {
            const selection = view.state.selection.main;
            if (!selection.empty) return false;
            const link = previewLinks(view).find(
              (link) => selection.head === (direction < 0 ? link.to : link.from)
            );
            if (!link) return false;
            view.dispatch({ selection: EditorSelection.cursor(selection.head + direction) });
            return true;
          }
        }))
      )
    )
  ];
}
