import { searchEmojis, type EmojiResult } from '$lib/emoji';
import { emojiShortcodeQuery } from '@chatto/client/messaging/emojiShortcodes';
import type { ComposerEditorApi } from './editorTypes';

export type MentionRole = {
  name: string;
  isSystem?: boolean;
  position?: number;
  pingable?: boolean;
};

/** Query text and the position of its trigger in the editor. */
type AutocompleteQuery = {
  query: string;
  triggerStart: number;
};

export class AutocompleteState {
  /** The same matches control popup visibility and populate its list. */
  emoji = $state.raw<(AutocompleteQuery & { results: EmojiResult[] }) | null>(null);
  emojiRef = $state<{ handleKeyDown: (e: KeyboardEvent) => boolean } | null>(null);
  mention = $state<AutocompleteQuery | null>(null);
  mentionRef = $state<{ handleKeyDown: (e: KeyboardEvent) => boolean } | null>(null);
  /** Active room query; only message composers enable room links. */
  room = $state<AutocompleteQuery | null>(null);
  roomRef = $state<{ handleKeyDown: (e: KeyboardEvent) => boolean } | null>(null);

  constructor(
    private readonly getEditorApi: () => ComposerEditorApi | null,
    private readonly options: { openOnAt?: boolean; emojis?: boolean; rooms?: boolean } = {}
  ) {}

  reset(): void {
    this.emoji = null;
    this.mention = null;
    this.room = null;
  }

  resetForRoom(): void {
    this.reset();
  }

  update(): void {
    this.updateEmoji();
    this.updateMention();
    this.updateRoom();
  }

  closeRoom(): void {
    this.room = null;
  }

  /** Insert the selected room URL with a trailing space. */
  selectRoom(url: string): void {
    if (!this.room) return;
    this.applyCompletion(url, this.room.triggerStart);
    this.room = null;
  }

  private updateRoom(): void {
    const editorApi = this.getEditorApi();
    if (!this.options.rooms || !editorApi || editorApi.isInCodeBlock()) {
      this.room = null;
      return;
    }
    const text = editorApi.getTextBeforeCursor();
    // Whitespace ends a dismissed query. Embedded URL fragments do not open a picker.
    const match = /\s$/u.test(text) ? null : text.match(/(?:^|\s)#([^\s#]*)$/u);
    this.room = match ? { query: match[1], triggerStart: text.length - match[1].length - 1 } : null;
    if (this.room) {
      this.emoji = null;
      this.mention = null;
    }
  }

  closeEmoji(): void {
    this.emoji = null;
  }

  closeMention(): void {
    this.mention = null;
  }

  /** Insert the selected emoji with a trailing space and close its suggestions. */
  selectEmoji(emoji: string): void {
    if (!this.emoji) return;
    this.applyCompletion(emoji, this.emoji.triggerStart);
    this.emoji = null;
  }

  /** Insert the selected @handle with a trailing space and close its suggestions. */
  selectMention(handle: string): void {
    if (!this.mention) return;

    this.applyCompletion('@' + handle, this.mention.triggerStart);
    this.mention = null;
  }

  private updateEmoji(): void {
    if (this.options.emojis === false) return;
    const partial = this.getEmojiPartialAtCursor();
    // Keep the result identity so edits outside the query preserve keyboard selection.
    const results = partial
      ? partial.query === this.emoji?.query
        ? this.emoji.results
        : searchEmojis(partial.query, Infinity)
      : [];
    if (partial && results.length > 0) {
      this.emoji = {
        query: partial.query,
        triggerStart: partial.start,
        results
      };
      this.mention = null;
    } else {
      this.emoji = null;
    }
  }

  private updateMention(): void {
    if (this.emoji) {
      this.mention = null;
      return;
    }

    const partial = this.getMentionPartialAtCursor();
    if (partial) {
      this.mention = {
        query: partial.partial,
        triggerStart: partial.start
      };
    } else {
      this.mention = null;
    }
  }

  private getEmojiPartialAtCursor(): { query: string; start: number } | null {
    const editorApi = this.getEditorApi();
    if (!editorApi) return null;

    const match = emojiShortcodeQuery(editorApi.getTextBeforeCursor());
    if (!match || match.name.length < 2) return null;

    return {
      query: match.name,
      start: match.start
    };
  }

  private getMentionPartialAtCursor(): { partial: string; start: number } | null {
    const editorApi = this.getEditorApi();
    if (!editorApi) return null;

    const textBefore = editorApi.getTextBeforeCursor();
    const match = textBefore.match(/(?:^|[\s])@([a-zA-Z0-9_.-]*)$/);
    if (!match) return null;
    if (!match[1] && !this.options.openOnAt) return null;

    return {
      partial: match[1],
      start: textBefore.length - match[1].length - 1
    };
  }

  /** Replace the active query with the choice and a space; preserve text after the cursor. */
  private applyCompletion(text: string, atPosition: number): void {
    const editorApi = this.getEditorApi();
    if (!editorApi) return;

    const textBefore = editorApi.getTextBeforeCursor();
    const charsToReplace = textBefore.length - atPosition;
    editorApi.replaceTextBeforeCursor(charsToReplace, text + ' ');
  }
}
