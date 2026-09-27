import type { RoomMember } from '$lib/state/room';
import { searchEmojis } from '$lib/emoji';
import type { ComposerEditorApi } from './editorTypes';
import { rankMentionCandidates } from './mentionRanking';

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
    private readonly getEditorApi: () => ComposerEditorApi | null,
    private readonly getMembers: () => RoomMember[],
    private readonly getRoles: () => MentionRole[] = () => [],
    /** Users that rank first among matches, such as thread participants. */
    private readonly getPrioritizedUserIds?: () => ReadonlySet<string> | undefined
  ) {}

  reset(): void {
    this.emoji = null;
    this.mention = null;
    this.tabCompletion = null;
  }

  resetForRoom(): void {
    this.reset();
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
    if (partial) {
      this.mention = {
        query: partial.partial,
        triggerStart: partial.start
      };
    } else {
      this.mention = null;
    }
  }

  private findMatchingMentions(partial: string): string[] {
    return rankMentionCandidates(
      partial,
      this.getMembers(),
      this.getRoles(),
      this.getPrioritizedUserIds?.()
    ).map((result) => result.handle);
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
