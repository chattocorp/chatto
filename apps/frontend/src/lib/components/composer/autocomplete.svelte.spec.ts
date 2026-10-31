import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { describe, it, expect, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { searchEmojis } from '$lib/emoji';
import EmojiAutocomplete from './EmojiAutocomplete.svelte';

vi.mock('$lib/emoji', { spy: true });

import type { RoomMember } from '$lib/state/room';
import type { ComposerEditorApi } from './editorTypes';
import { AutocompleteState } from './autocomplete.svelte';

function member(login: string, displayName = login, deleted = false): RoomMember {
  return {
    id: `user_${login}`,
    login,
    displayName,
    deleted,
    avatarUrl: null,
    presenceStatus: PresenceStatus.OFFLINE
  };
}

function editor(
  initialText: string,
  initialCursor = initialText.length
): {
  api: ComposerEditorApi;
  getText: () => string;
  setText: (text: string) => void;
  setCursor: (position: number) => void;
} {
  let text = initialText;
  let cursor = initialCursor;
  return {
    api: {
      getText: () => text,
      setContent: (next) => {
        text = next;
        cursor = text.length;
      },
      focus: () => {},
      performEnter: () => {},
      getTextBeforeCursor: () => text.slice(0, cursor),
      isInCodeBlock: () => false,
      replaceTextBeforeCursor: (charCount, replacement) => {
        text = text.slice(0, cursor - charCount) + replacement + text.slice(cursor);
        cursor = cursor - charCount + replacement.length;
      },
      insertText: (inserted) => {
        text = text.slice(0, cursor) + inserted + text.slice(cursor);
        cursor += inserted.length;
      },
      toggleFormatting: () => {},
      adjustIndent: () => false,
      insertQuote: () => {}
    },
    getText: () => text,
    setText: (next) => {
      text = next;
      cursor = text.length;
    },
    setCursor: (position) => {
      cursor = position;
    }
  };
}

function tabEvent(): KeyboardEvent {
  return new KeyboardEvent('keydown', { key: 'Tab', cancelable: true });
}

describe('AutocompleteState', () => {
  it('searches once for both popup visibility and its ranked, limited results', () => {
    const fakeEditor = editor(':he');
    const state = new AutocompleteState(
      () => fakeEditor.api,
      () => []
    );
    vi.mocked(searchEmojis).mockClear();
    state.update();
    expect(state.emoji?.results).toHaveLength(10);
    const { container } = render(EmojiAutocomplete, {
      query: state.emoji!.query,
      results: state.emoji!.results,
      onSelect: () => {},
      onClose: () => {}
    });
    expect(searchEmojis).toHaveBeenCalledExactlyOnceWith('he', 10);
    expect(container.querySelectorAll('button')).toHaveLength(10);

    fakeEditor.setText(':zzzznothing');
    state.update();
    expect(state.emoji).toBeNull();
  });

  it.each(['Enter', 'Tab', 'click'])(
    'uses the %s selection whitespace rule for all pickers',
    (key) => {
      const fakeEditor = editor('See #gen! later', 8);
      const state = new AutocompleteState(
        () => fakeEditor.api,
        () => [member('alice')],
        undefined,
        undefined,
        { rooms: true }
      );
      const suffix = key === 'click' ? ' ' : '';
      state.update();
      state.selectRoom('https://chat.example/room', key);
      expect(fakeEditor.getText()).toBe(`See https://chat.example/room${suffix}! later`);
      expect(state.room).toBeNull();
      fakeEditor.setText('@ali');
      state.update();
      state.selectMention('alice', key);
      expect(fakeEditor.getText()).toBe(`@alice${suffix}`);
      expect(state.mention).toBeNull();
      fakeEditor.setText(':fi');
      state.update();
      state.selectEmoji('🔥', key);
      expect(fakeEditor.getText()).toBe(`🔥${suffix}`);
      expect(state.emoji).toBeNull();
    }
  );
  it('replaces only the room query and resets on dismissal and room changes', () => {
    const fakeEditor = editor('See #gen tomorrow', 8);
    const state = new AutocompleteState(
      () => fakeEditor.api,
      () => [],
      undefined,
      undefined,
      { rooms: true }
    );
    state.update();
    expect(state.room).toEqual({ query: 'gen', triggerStart: 4 });
    state.selectRoom('https://chat.example/chat/-/R12345678901234', 'click');
    expect(fakeEditor.getText()).toBe('See https://chat.example/chat/-/R12345678901234  tomorrow');
    expect(state.room).toBeNull();
    fakeEditor.setText('#');
    state.update();
    expect(state.room?.query).toBe('');
    state.closeRoom();
    expect(state.room).toBeNull();
    state.update();
    state.resetForRoom();
    expect(state.room).toBeNull();
  });

  it.each(['# heading', '##', 'word#room', 'https://example.com/#room', '#room\n'])(
    'does not open a room picker for headings or embedded hashes: %s',
    (text) => {
      const fakeEditor = editor(text);
      const state = new AutocompleteState(
        () => fakeEditor.api,
        () => [],
        undefined,
        undefined,
        { rooms: true }
      );
      state.update();
      expect(state.room).toBeNull();
    }
  );

  it('keeps room completion disabled in other fields and in code blocks', () => {
    const fakeEditor = editor('#general');
    const disabled = new AutocompleteState(
      () => fakeEditor.api,
      () => []
    );
    disabled.update();
    expect(disabled.room).toBeNull();
    fakeEditor.api.isInCodeBlock = () => true;
    const state = new AutocompleteState(
      () => fakeEditor.api,
      () => [],
      undefined,
      undefined,
      { rooms: true }
    );
    state.update();
    expect(state.room).toBeNull();
  });

  it.each(['(:he', '!:he', ':wave::he', '👋:he', ':heart: :he'])(
    'opens and completes emoji after a valid boundary: %s',
    (text) => {
      const fakeEditor = editor(text);
      const state = new AutocompleteState(
        () => fakeEditor.api,
        () => []
      );
      state.update();
      expect(state.emoji?.query).toBe('he');
      state.selectEmoji('❤️', 'click');
      expect(fakeEditor.getText()).toBe(text.slice(0, -3) + '❤️ ');
    }
  );

  it.each(['hello:he', '_:he', 'é:he', '中:he', '𐐀:he', 'e\u0301:he', ':he\n'])(
    'does not open emoji autocomplete inside words: %s',
    (text) => {
      const fakeEditor = editor(text);
      const state = new AutocompleteState(
        () => fakeEditor.api,
        () => []
      );
      state.update();
      expect(state.emoji).toBeNull();
    }
  );

  it.each([':+1', ':-1'])('supports signed emoji aliases: %s', (text) => {
    const fakeEditor = editor(text);
    const state = new AutocompleteState(
      () => fakeEditor.api,
      () => []
    );
    state.update();
    expect(state.emoji?.query).toBe(text.slice(1));
  });

  it('shows mention autocomplete when an @ partial has characters, even before members load', () => {
    const fakeEditor = editor('@');
    const state = new AutocompleteState(
      () => fakeEditor.api,
      () => []
    );

    state.update();
    expect(state.mention).toBeNull();

    fakeEditor.setText('@a');
    state.update();
    expect(state.mention?.query).toBe('a');

    fakeEditor.setText('@');
    state.update();
    expect(state.mention).toBeNull();
  });

  it('shows emoji autocomplete only after the two-character shortcode threshold', () => {
    const fakeEditor = editor(':');
    const state = new AutocompleteState(
      () => fakeEditor.api,
      () => []
    );

    state.update();
    expect(state.emoji).toBeNull();

    fakeEditor.setText(':h');
    state.update();
    expect(state.emoji).toBeNull();

    fakeEditor.setText(':he');
    state.update();
    expect(state.emoji?.query).toBe('he');

    fakeEditor.setText(':h');
    state.update();
    expect(state.emoji).toBeNull();
  });

  it('gives emoji autocomplete priority over mention autocomplete', () => {
    const fakeEditor = editor('@al');
    const state = new AutocompleteState(
      () => fakeEditor.api,
      () => [member('alice')]
    );

    state.update();
    expect(state.mention?.query).toBe('al');
    expect(state.emoji).toBeNull();

    fakeEditor.setText('@al :fi');
    state.update();

    expect(state.emoji?.query).toBe('fi');
    expect(state.mention).toBeNull();
  });

  it('cycles mention Tab completion and resets the cycle on another key', () => {
    const fakeEditor = editor('@ali');
    const state = new AutocompleteState(
      () => fakeEditor.api,
      () => [member('alice'), member('alicia')],
      () => []
    );

    const firstTab = tabEvent();
    expect(state.handleTabCompletion(firstTab)).toBe(true);
    expect(firstTab.defaultPrevented).toBe(true);
    expect(fakeEditor.getText()).toBe('@alice');

    expect(state.handleTabCompletion(tabEvent())).toBe(true);
    expect(fakeEditor.getText()).toBe('@alicia');

    state.resetTabCompletion();
    fakeEditor.setText('@alicia ');
    expect(state.handleTabCompletion(tabEvent())).toBe(false);
    expect(fakeEditor.getText()).toBe('@alicia ');
  });

  it('completes prioritized users first on Tab', () => {
    const fakeEditor = editor('@ali');
    const state = new AutocompleteState(
      () => fakeEditor.api,
      () => [member('alice'), member('alicia')],
      () => [],
      () => new Set(['user_alicia'])
    );

    expect(state.handleTabCompletion(tabEvent())).toBe(true);
    expect(fakeEditor.getText()).toBe('@alicia');

    expect(state.handleTabCompletion(tabEvent())).toBe(true);
    expect(fakeEditor.getText()).toBe('@alice');
  });

  it('excludes deleted members from mention Tab completion', () => {
    const fakeEditor = editor('@deleted');
    const state = new AutocompleteState(
      () => fakeEditor.api,
      () => [member('', 'Deleted User', true), member('deleted-alice', 'Deleted Alice', true)]
    );
    const ev = tabEvent();

    expect(state.handleTabCompletion(ev)).toBe(true);
    expect(fakeEditor.getText()).toBe('@deleted');
  });

  it('does not handle Tab when there is no mention partial at the cursor', () => {
    const fakeEditor = editor('hello world');
    const state = new AutocompleteState(
      () => fakeEditor.api,
      () => [member('alice')]
    );
    const ev = tabEvent();

    expect(state.handleTabCompletion(ev)).toBe(false);
    expect(ev.defaultPrevented).toBe(false);
    expect(fakeEditor.getText()).toBe('hello world');
  });

  it('does not handle Tab when the cursor is after a bare @', () => {
    const fakeEditor = editor('@');
    const state = new AutocompleteState(
      () => fakeEditor.api,
      () => [member('alice')]
    );
    const ev = tabEvent();

    expect(state.handleTabCompletion(ev)).toBe(false);
    expect(ev.defaultPrevented).toBe(false);
    expect(fakeEditor.getText()).toBe('@');
  });

  it('selects a mention from the popup and seeds Tab cycling only for Tab selection', () => {
    const fakeEditor = editor('@ali');
    const state = new AutocompleteState(
      () => fakeEditor.api,
      () => [member('alice'), member('alicia')],
      () => []
    );

    state.update();
    state.selectMention('alice', 'Enter');

    expect(fakeEditor.getText()).toBe('@alice');
    expect(state.tabCompletion).toBeNull();

    fakeEditor.setText('@ali');
    state.update();
    state.selectMention('alice', 'Tab');

    expect(state.tabCompletion?.candidates).toEqual(['alice', 'alicia']);
  });

  it('completes a mention after leading text', () => {
    const fakeEditor = editor('Hey @ali');
    const state = new AutocompleteState(
      () => fakeEditor.api,
      () => [member('alice')]
    );

    const ev = tabEvent();
    expect(state.handleTabCompletion(ev)).toBe(true);
    expect(fakeEditor.getText()).toBe('Hey @alice');
  });

  it('completes virtual and role mention handles', () => {
    const fakeEditor = editor('@he');
    const state = new AutocompleteState(
      () => fakeEditor.api,
      () => [member('helena')],
      () => [{ name: 'helpdesk', pingable: true }]
    );

    state.update();
    expect(state.mention?.query).toBe('he');

    const firstTab = tabEvent();
    expect(state.handleTabCompletion(firstTab)).toBe(true);
    expect(fakeEditor.getText()).toBe('@helena');

    expect(state.handleTabCompletion(tabEvent())).toBe(true);
    expect(fakeEditor.getText()).toBe('@here');

    expect(state.handleTabCompletion(tabEvent())).toBe(true);
    expect(fakeEditor.getText()).toBe('@helpdesk');
  });

  it('selects an emoji by replacing the shortcode before the cursor', () => {
    const fakeEditor = editor('hello :fi');
    const state = new AutocompleteState(
      () => fakeEditor.api,
      () => []
    );

    state.update();
    state.selectEmoji('🔥', 'click');

    expect(fakeEditor.getText()).toBe('hello 🔥 ');
    expect(state.emoji).toBeNull();
  });
});
