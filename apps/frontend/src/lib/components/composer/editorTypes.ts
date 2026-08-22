import type { QuoteInsertionContent } from '$lib/state/room';

export type ComposerFormattingCommand =
  | 'bold'
  | 'italic'
  | 'inlineCode'
  | 'heading'
  | 'bulletList'
  | 'orderedList'
  | 'blockquote'
  | 'codeBlock';

export type ComposerFormattingState = Record<ComposerFormattingCommand, boolean>;

export type ComposerIndentDirection = 'indent' | 'outdent';

export type ComposerIndentState = {
  canIndent: boolean;
  canOutdent: boolean;
};

export const emptyComposerIndentState: ComposerIndentState = {
  canIndent: false,
  canOutdent: false
};

export type ComposerEditorApi = {
  /** Get the editor's current text content for empty-state checks. */
  getText: () => string;
  /** Set editor content from Markdown. */
  setContent: (markdown: string) => void;
  /** Focus the editor. */
  focus: (position?: 'start' | 'end') => void;
  /** Perform the editor's normal, context-sensitive Enter action. */
  performEnter: () => void;
  /** Get plain text from document start to cursor position. */
  getTextBeforeCursor: () => string;
  /** Whether the current selection is inside a code block. */
  isInCodeBlock: () => boolean;
  /**
   * Replace N characters before the cursor with new text.
   * Used for mention/emoji completion where the pattern length relative to the
   * cursor is known.
   */
  replaceTextBeforeCursor: (charCount: number, replacement: string) => void;
  /** Insert plain text at the current cursor position. */
  insertText: (text: string) => void;
  /** Toggle a Markdown formatting command at the current selection. */
  toggleFormatting: (command: ComposerFormattingCommand) => void;
  /** Apply the editor's native indent or outdent action. */
  adjustIndent: (direction: ComposerIndentDirection) => boolean;
  /** Insert selected reply text as a blockquote at the current cursor. */
  insertQuote: (text: QuoteInsertionContent) => void;
};

export type ComposerEditorProps = {
  placeholder?: string;
  editable?: boolean;
  autofocus?: boolean;
  testid?: string;
  onUpdate?: (markdown: string) => void;
  onKeyDown?: (event: KeyboardEvent) => boolean;
  onPaste?: (event: ClipboardEvent) => boolean;
  onFormattingStateChange?: (state: ComposerFormattingState) => void;
  onIndentStateChange?: (state: ComposerIndentState) => void;
  onReady?: (api: ComposerEditorApi) => void;
  onDestroy?: (api: ComposerEditorApi) => void;
};
