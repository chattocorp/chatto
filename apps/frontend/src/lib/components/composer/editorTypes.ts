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

export type TipTapEditorApi = {
  /** Get the editor's plain text content. */
  getText: () => string;
  /** Set editor content from Markdown. */
  setContent: (markdown: string) => void;
  /** Focus the editor. */
  focus: (position?: 'start' | 'end') => void;
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
  /** Insert selected reply text as a blockquote at the current cursor. */
  insertQuote: (text: QuoteInsertionContent) => void;
  /** Insert the same block break the editor would create for a plain Enter key. */
  insertBlockBreak: () => void;
};
