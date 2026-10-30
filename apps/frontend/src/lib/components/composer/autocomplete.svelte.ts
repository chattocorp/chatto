import type { RoomMember } from '$lib/state/room';
import { searchEmojis } from '$lib/emoji';
import { emojiShortcodeQuery } from '@chatto/client/messaging/emojiShortcodes';
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
};

/** Query text and the position of its trigger in the editor. */
type AutocompleteQuery = {
  query: string;
  triggerStart: number;
};

export class AutocompleteState {
  tabCompletion = $state<TabCompletionState | null>(null);
  emoji = $state<AutocompleteQuery | null>(null);
  emojiRef = $state<{ handleKeyDown: (e: KeyboardEvent) => boolean } | null>(null);
  mention = $state<AutocompleteQuery | null>(null);
  mentionRef = $state<{ handleKeyDown: (e: KeyboardEvent) => boolean } | null>(null);
  /** Active room query; only message composers enable room links. */
  room = $state<AutocompleteQuery | null>(null);
  roomRef = $state<{ handleKeyDown: (e: KeyboardEvent) => boolean } | null>(null);

  constructor(
    private readonly getEditorApi: () => ComposerEditorApi | null,
    private readonly getMembers: () => RoomMember[],
    private readonly getRoles: () => MentionRole[] = () => [],
    /** Users that rank first among matches, such as thread participants. */
    private readonly getPrioritizedUserIds?: () => ReadonlySet<string> | undefined,
    private readonly options: { openOnAt?: boolean; emojis?: boolean; rooms?: boolean } = {}
  ) {}

  reset(): void {
    this.emoji = null;
    this.mention = null;
    this.room = null;
    this.tabCompletion = null;
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

  /** Space and pointer selection add a space; Enter and Tab insert only the URL. */
  selectRoom(url: string, key: string): void {
    if (!this.room) return;
    this.applyCompletion(url, this.room.triggerStart, completionSuffix(key));
    this.room = null;
    this.tabCompletion = null;
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

  selectEmoji(emoji: string, key: string): void {
    if (!this.emoji) return;
    this.applyCompletion(emoji, this.emoji.triggerStart, completionSuffix(key));
    this.emoji = null;
  }

  selectMention(handle: string, key: string): void {
    if (!this.mention) return;

    const triggerStart = this.mention.triggerStart;
    const originalPartial = this.mention.query;

    this.applyCompletion('@' + handle, triggerStart, completionSuffix(key));
    this.mention = null;

    this.tabCompletion = null;
    if (key !== 'Tab') return;

    const candidates = this.findMatchingMentions(originalPartial);
    if (candidates.length > 1) {
      const selectedIdx = candidates.indexOf(handle);
      this.tabCompletion = {
        candidates,
        index: selectedIdx >= 0 ? selectedIdx : 0,
        triggerStart
      };
    }
  }

  handleTabCompletion(event: KeyboardEvent): boolean {
    const editorApi = this.getEditorApi();
    if (!editorApi) return false;

    if (this.tabCompletion && this.tabCompletion.candidates.length > 1) {
      const currentHandle = this.tabCompletion.candidates[this.tabCompletion.index];
      const expectedCursorPos = this.tabCompletion.triggerStart + 1 + currentHandle.length;
      const currentPos = editorApi.getTextBeforeCursor().length;

      if (currentPos === expectedCursorPos) {
        event.preventDefault();
        const nextIndex = (this.tabCompletion.index + 1) % this.tabCompletion.candidates.length;
        this.tabCompletion = { ...this.tabCompletion, index: nextIndex };
        this.applyCompletion(
          '@' + this.tabCompletion.candidates[nextIndex],
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
        triggerStart: mentionInfo.start
      };
      this.applyCompletion('@' + candidates[0], mentionInfo.start);
    }

    return true;
  }

  resetTabCompletion(): void {
    this.tabCompletion = null;
  }

  private updateEmoji(): void {
    if (this.options.emojis === false) return;
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

  /** Replace just the active query; preserve all text after the cursor. */
  private applyCompletion(text: string, atPosition: number, suffix = ''): void {
    const editorApi = this.getEditorApi();
    if (!editorApi) return;

    const textBefore = editorApi.getTextBeforeCursor();
    const charsToReplace = textBefore.length - atPosition;
    editorApi.replaceTextBeforeCursor(charsToReplace, text + suffix);
  }
}

/** Keep pointer selection convenient; keyboard selection adds only the requested space. */
function completionSuffix(key: string): string {
  return key === ' ' || key === 'click' ? ' ' : '';
}
