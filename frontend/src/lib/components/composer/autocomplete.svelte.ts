import type { RoomMember } from '$lib/state/room';
import { fuzzyMatch } from '$lib/fuzzyMatch';
import { searchEmojis } from '$lib/emoji';
import type { TipTapEditorApi } from './TipTapEditor.svelte';

type TabCompletionState = {
  candidates: string[];
  index: number;
  triggerStart: number;
  originalPartial: string;
};

export type EmojiAutocompleteState = {
  query: string;
  triggerStart: number;
};

export type MentionAutocompleteState = {
  query: string;
  triggerStart: number;
};

export class AutocompleteState {
  tabCompletion = $state<TabCompletionState | null>(null);
  emoji = $state<EmojiAutocompleteState | null>(null);
  emojiRef = $state<{ handleKeyDown: (e: KeyboardEvent) => boolean } | null>(null);
  mention = $state<MentionAutocompleteState | null>(null);
  mentionRef = $state<{ handleKeyDown: (e: KeyboardEvent) => boolean } | null>(null);

  constructor(
    private readonly getEditorApi: () => TipTapEditorApi | null,
    private readonly getMembers: () => RoomMember[]
  ) {}

  resetForRoom(): void {
    this.emoji = null;
    this.mention = null;
    this.tabCompletion = null;
  }

  update(): void {
    this.updateEmoji();
    this.updateMention();
  }

  closeEmoji(): void {
    this.emoji = null;
  }

  closeMention(): void {
    this.mention = null;
  }

  selectEmoji(emoji: string): void {
    if (!this.emoji) return;
    const editorApi = this.getEditorApi();
    if (!editorApi) return;

    const textBefore = editorApi.getTextBeforeCursor();
    const charsToReplace = textBefore.length - this.emoji.triggerStart;
    editorApi.replaceTextBeforeCursor(charsToReplace, emoji + ' ');
    this.emoji = null;
  }

  selectMention(login: string, viaTab: boolean): void {
    if (!this.mention) return;

    const triggerStart = this.mention.triggerStart;
    const originalPartial = this.mention.query;

    this.applyCompletion(login, triggerStart);
    this.mention = null;

    if (!viaTab) return;

    const candidates = this.findMatchingMembers(originalPartial);
    if (candidates.length > 1) {
      const selectedIdx = candidates.indexOf(login);
      this.tabCompletion = {
        candidates,
        index: selectedIdx >= 0 ? selectedIdx : 0,
        triggerStart,
        originalPartial
      };
    }
  }

  handleTabCompletion(event: KeyboardEvent): boolean {
    const editorApi = this.getEditorApi();
    if (!editorApi) return false;

    if (this.tabCompletion && this.tabCompletion.candidates.length > 1) {
      const currentUsername = this.tabCompletion.candidates[this.tabCompletion.index];
      const expectedCursorPos = this.tabCompletion.triggerStart + 1 + currentUsername.length + 1;
      const currentPos = editorApi.getTextBeforeCursor().length;

      if (currentPos === expectedCursorPos) {
        event.preventDefault();
        const nextIndex = (this.tabCompletion.index + 1) % this.tabCompletion.candidates.length;
        this.tabCompletion = { ...this.tabCompletion, index: nextIndex };
        this.applyCompletion(this.tabCompletion.candidates[nextIndex], this.tabCompletion.triggerStart);
        return true;
      }
    }

    const mentionInfo = this.getMentionPartialAtCursor();
    if (!mentionInfo || mentionInfo.partial.length === 0) return false;

    event.preventDefault();

    const candidates = this.findMatchingMembers(mentionInfo.partial);
    if (candidates.length > 0) {
      this.tabCompletion = {
        candidates,
        index: 0,
        triggerStart: mentionInfo.start,
        originalPartial: mentionInfo.partial
      };
      this.applyCompletion(candidates[0], mentionInfo.start);
    }

    return true;
  }

  resetTabCompletion(): void {
    this.tabCompletion = null;
  }

  private updateEmoji(): void {
    const partial = this.getEmojiPartialAtCursor();
    if (partial && searchEmojis(partial.query, 1).length > 0) {
      this.emoji = {
        query: partial.query,
        triggerStart: partial.start
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
    if (partial && this.findMatchingMembers(partial.partial).length > 0) {
      this.mention = {
        query: partial.partial,
        triggerStart: partial.start
      };
    } else {
      this.mention = null;
    }
  }

  private findMatchingMembers(partial: string): string[] {
    const scored: { login: string; score: number }[] = [];

    for (const m of this.getMembers()) {
      const loginScore = fuzzyMatch(partial, m.login);
      const displayScore = fuzzyMatch(partial, m.displayName);
      const bestScore = Math.max(loginScore ?? -1, displayScore ?? -1);

      if (bestScore > 0) {
        scored.push({ login: m.login, score: bestScore });
      }
    }

    scored.sort((a, b) => b.score - a.score);
    return scored.map((s) => s.login);
  }

  private getEmojiPartialAtCursor(): { query: string; start: number } | null {
    const editorApi = this.getEditorApi();
    if (!editorApi) return null;

    const textBefore = editorApi.getTextBeforeCursor();
    const match = textBefore.match(/(?:^|[\s]):([\w]{2,})$/);
    if (!match) return null;

    return {
      query: match[1],
      start: textBefore.length - match[1].length - 1
    };
  }

  private getMentionPartialAtCursor(): { partial: string; start: number } | null {
    const editorApi = this.getEditorApi();
    if (!editorApi) return null;

    const textBefore = editorApi.getTextBeforeCursor();
    const match = textBefore.match(/(?:^|[\s])@([a-zA-Z0-9_.-]+)$/);
    if (!match) return null;

    return {
      partial: match[1],
      start: textBefore.length - match[1].length - 1
    };
  }

  private applyCompletion(username: string, atPosition: number): void {
    const editorApi = this.getEditorApi();
    if (!editorApi) return;

    const textBefore = editorApi.getTextBeforeCursor();
    const charsToReplace = textBefore.length - atPosition;
    editorApi.replaceTextBeforeCursor(charsToReplace, '@' + username + ' ');
  }
}
