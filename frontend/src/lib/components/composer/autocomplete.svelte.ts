import type { RoomMember } from '$lib/state/room';
import { fuzzyMatch } from '$lib/fuzzyMatch';
import { searchEmojis } from '$lib/emoji';
import type { TipTapEditorApi } from './TipTapEditor.svelte';

export type MentionRole = {
  name: string;
  isSystem?: boolean;
  position?: number;
  pingable?: boolean;
};

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
    private readonly getMembers: () => RoomMember[],
    private readonly getRoles: () => MentionRole[] = () => []
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

  selectMention(handle: string, viaTab: boolean): void {
    if (!this.mention) return;

    const triggerStart = this.mention.triggerStart;
    const originalPartial = this.mention.query;

    this.applyCompletion(handle, triggerStart);
    this.mention = null;

    if (!viaTab) return;

    const candidates = this.findMatchingMentions(originalPartial);
    if (candidates.length > 1) {
      const selectedIdx = candidates.indexOf(handle);
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
      const currentHandle = this.tabCompletion.candidates[this.tabCompletion.index];
      const expectedCursorPos = this.tabCompletion.triggerStart + 1 + currentHandle.length + 1;
      const currentPos = editorApi.getTextBeforeCursor().length;

      if (currentPos === expectedCursorPos) {
        event.preventDefault();
        const nextIndex = (this.tabCompletion.index + 1) % this.tabCompletion.candidates.length;
        this.tabCompletion = { ...this.tabCompletion, index: nextIndex };
        this.applyCompletion(
          this.tabCompletion.candidates[nextIndex],
          this.tabCompletion.triggerStart
        );
        return true;
      }
    }

    const mentionInfo = this.getMentionPartialAtCursor();
    if (!mentionInfo || mentionInfo.partial.length === 0) return false;

    event.preventDefault();

    const candidates = this.findMatchingMentions(mentionInfo.partial);
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
    if (partial && this.findMatchingMentions(partial.partial).length > 0) {
      this.mention = {
        query: partial.partial,
        triggerStart: partial.start
      };
    } else {
      this.mention = null;
    }
  }

  private findMatchingMentions(partial: string): string[] {
    const scored: { handle: string; score: number; priority: number }[] = [];

    for (const m of this.getMembers()) {
      if (m.deleted || !m.login) continue;

      const loginScore = fuzzyMatch(partial, m.login);
      const displayScore = fuzzyMatch(partial, m.displayName);
      const bestScore = Math.max(loginScore ?? -1, displayScore ?? -1);

      if (bestScore > 0) {
        scored.push({ handle: m.login, score: bestScore, priority: 0 });
      }
    }

    for (const target of ['all', 'here']) {
      const score = fuzzyMatch(partial, target);
      if (score && score > 0) {
        scored.push({ handle: target, score, priority: 1 });
      }
    }

    for (const role of this.getRoles()) {
      if (!role.pingable || role.name === 'everyone') continue;
      const score = fuzzyMatch(partial, role.name);
      if (score && score > 0) {
        scored.push({ handle: role.name, score, priority: 2 });
      }
    }

    scored.sort(
      (a, b) => a.priority - b.priority || b.score - a.score || a.handle.localeCompare(b.handle)
    );
    return scored.map((s) => s.handle);
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

  private applyCompletion(handle: string, atPosition: number): void {
    const editorApi = this.getEditorApi();
    if (!editorApi) return;

    const textBefore = editorApi.getTextBeforeCursor();
    const charsToReplace = textBefore.length - atPosition;
    editorApi.replaceTextBeforeCursor(charsToReplace, '@' + handle + ' ');
  }
}
