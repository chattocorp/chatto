import { syntaxTree } from '@codemirror/language';
import {
  StateEffect,
  StateField,
  type EditorState,
  type Extension,
  type Range
} from '@codemirror/state';
import {
  Decoration,
  EditorView,
  ViewPlugin,
  type DecorationSet,
  type ViewUpdate
} from '@codemirror/view';

type HighlightTreeNode =
  | { type: 'text'; value: string }
  | {
      type: 'element';
      properties?: { className?: unknown };
      children?: HighlightTreeNode[];
    };

type CodeHighlightingModule = typeof import('$lib/codeHighlighting');

type CodeFence = {
  language: string | null;
  code: string;
  from: number;
  to: number;
  blockFrom: number;
  blockTo: number;
  closed: boolean;
};

type HighlightSpan = {
  from: number;
  to: number;
  classes: string;
};

const plainTextLanguages = new Set(['text', 'txt', 'plain', 'plaintext']);
const setCodeHighlights = StateEffect.define<DecorationSet>();
let codeHighlightingPromise: Promise<CodeHighlightingModule> | null = null;

const codeHighlightState = StateField.define<DecorationSet>({
  create: () => Decoration.none,
  update: (decorations, transaction) => {
    if (transaction.docChanged) decorations = Decoration.none;
    for (const effect of transaction.effects) {
      if (effect.is(setCodeHighlights)) decorations = effect.value;
    }
    return decorations;
  },
  provide: (field) => EditorView.decorations.from(field)
});

const codeBodyState = StateField.define<DecorationSet>({
  create: (state) => codeBodyDecorations(state),
  update: (decorations, transaction) =>
    transaction.docChanged ? codeBodyDecorations(transaction.state) : decorations,
  provide: (field) => EditorView.decorations.from(field)
});

function normalizedLanguageToken(value: string): string {
  return (
    value
      .trim()
      .toLowerCase()
      .match(/[a-z0-9+#_.-]+/)?.[0] ?? ''
  );
}

function collectCodeFences(state: EditorState): CodeFence[] {
  const fences: CodeFence[] = [];

  syntaxTree(state).iterate({
    enter(node) {
      if (node.name !== 'FencedCode') return;
      const languageNode = node.node.getChild('CodeInfo');
      const codeNode = node.node.getChild('CodeText');
      const codeMarks = node.node.getChildren('CodeMark');

      const language = languageNode
        ? normalizedLanguageToken(state.doc.sliceString(languageNode.from, languageNode.to))
        : null;

      fences.push({
        language: language && !plainTextLanguages.has(language) ? language : null,
        code: codeNode ? state.doc.sliceString(codeNode.from, codeNode.to) : '',
        from: codeNode?.from ?? node.to,
        to: codeNode?.to ?? node.to,
        blockFrom: node.from,
        blockTo: node.to,
        closed: codeMarks.length > 1
      });
    }
  });

  return fences;
}

function codeBodyDecorations(state: EditorState): DecorationSet {
  const ranges: Range<Decoration>[] = [];
  for (const fence of collectCodeFences(state)) {
    const firstLine = state.doc.lineAt(fence.blockFrom).number;
    const lastLine = state.doc.lineAt(fence.blockTo).number;
    for (let lineNumber = firstLine; lineNumber <= lastLine; lineNumber += 1) {
      const classes = ['cm-code-fence'];
      if (lineNumber === firstLine) classes.push('cm-code-fence-open');
      if (fence.closed && lineNumber === lastLine) classes.push('cm-code-fence-close');
      if (lineNumber > firstLine && (!fence.closed || lineNumber < lastLine)) {
        classes.push('cm-code-fence-body');
      }
      ranges.push(
        Decoration.line({ class: classes.join(' ') }).range(state.doc.line(lineNumber).from)
      );
    }
  }
  return Decoration.set(ranges, true);
}

/** Convert Lowlight's nested HAST token tree into non-overlapping CodeMirror spans. */
export function highlightTreeSpans(nodes: HighlightTreeNode[]): HighlightSpan[] {
  const spans: HighlightSpan[] = [];
  let offset = 0;

  function visit(node: HighlightTreeNode, inheritedClasses: string[]): void {
    if (node.type === 'text') {
      const from = offset;
      offset += node.value.length;
      if (inheritedClasses.length > 0 && offset > from) {
        spans.push({ from, to: offset, classes: inheritedClasses.join(' ') });
      }
      return;
    }

    const className = node.properties?.className;
    const ownClasses = Array.isArray(className)
      ? className.filter((value): value is string => typeof value === 'string')
      : typeof className === 'string'
        ? [className]
        : [];
    const classes = [...inheritedClasses, ...ownClasses];
    for (const child of node.children ?? []) visit(child, classes);
  }

  for (const node of nodes) visit(node, []);
  return spans;
}

async function getCodeHighlighting(): Promise<CodeHighlightingModule> {
  codeHighlightingPromise ??= import('$lib/codeHighlighting');
  return codeHighlightingPromise;
}

async function buildDecorations(fences: CodeFence[]): Promise<DecorationSet> {
  const highlightedFences = fences.filter(
    (fence): fence is CodeFence & { language: string } => fence.language !== null
  );
  if (highlightedFences.length === 0) return Decoration.none;

  const codeHighlighting = await getCodeHighlighting();
  await codeHighlighting.ensureCodeLanguagesLoaded(
    highlightedFences.map((fence) => fence.language)
  );

  const ranges = highlightedFences.flatMap((fence) => {
    const language = codeHighlighting.resolveCodeLanguage(fence.language);
    if (!language || !codeHighlighting.isCodeLanguageLoaded(language)) return [];

    const result = codeHighlighting.lowlight.highlight(language, fence.code) as {
      children: HighlightTreeNode[];
    };
    return highlightTreeSpans(result.children).map((span) =>
      Decoration.mark({ class: span.classes }).range(fence.from + span.from, fence.from + span.to)
    );
  });

  return Decoration.set(ranges, true);
}

const codeHighlightPlugin = ViewPlugin.fromClass(
  class {
    #generation = 0;
    #destroyed = false;

    constructor(view: EditorView) {
      this.#refresh(view);
    }

    update(update: ViewUpdate): void {
      if (update.docChanged) this.#refresh(update.view);
    }

    destroy(): void {
      this.#destroyed = true;
      this.#generation += 1;
    }

    #refresh(view: EditorView): void {
      const generation = ++this.#generation;
      const document = view.state.doc;
      const fences = collectCodeFences(view.state).filter((fence) => fence.language !== null);
      if (fences.length === 0) return;

      void buildDecorations(fences)
        .then((decorations) => {
          if (this.#destroyed || generation !== this.#generation || view.state.doc !== document)
            return;
          view.dispatch({ effects: setCodeHighlights.of(decorations) });
        })
        .catch((error: unknown) => {
          if (this.#destroyed || generation !== this.#generation || view.state.doc !== document)
            return;
          console.warn('[MarkdownEditor] Failed to highlight fenced code:', error);
        });
    }
  }
);

/** Highlight labelled Markdown code fences with Chatto's shared Lowlight registry. */
export const codeFenceHighlighting: Extension = [
  codeBodyState,
  codeHighlightState,
  codeHighlightPlugin
];
