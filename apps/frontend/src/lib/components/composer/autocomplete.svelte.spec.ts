import { describe, it, expect, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { searchEmojis } from '$lib/emoji';
import EmojiAutocomplete from './EmojiAutocomplete.svelte';

vi.mock('$lib/emoji', { spy: true });

import type { ComposerEditorApi } from './editorTypes';
import { AutocompleteState } from './autocomplete.svelte';

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

describe('AutocompleteState', () => {
  it('reuses matches for an unchanged query while updating the insertion offset', () => {
    const fakeEditor = editor(':he suffix', 3);
    const state = new AutocompleteState(() => fakeEditor.api);
    vi.mocked(searchEmojis).mockClear();
    state.update();
    const results = state.emoji!.results;
    fakeEditor.setText(':hesuffix');
    fakeEditor.setCursor(3);
    state.update();
    expect(state.emoji!.results).toBe(results);
    expect(searchEmojis).toHaveBeenCalledExactlyOnceWith('he', Infinity);
    // Reusing matches must not retain an old insertion offset.
    fakeEditor.setText('Hi :he');
    state.update();
    expect(state.emoji!.results).toBe(results);
    expect(state.emoji!.triggerStart).toBe(3);
    state.selectEmoji('❤️');
    expect(fakeEditor.getText()).toBe('Hi ❤️ ');
  });

  it('searches once for both popup visibility and all ranked results', () => {
    const fakeEditor = editor(':he');
    const state = new AutocompleteState(() => fakeEditor.api);
    vi.mocked(searchEmojis).mockClear();
    state.update();
    expect(state.emoji!.results.length).toBeGreaterThan(10);
    const { container } = render(EmojiAutocomplete, {
      query: state.emoji!.query,
      results: state.emoji!.results,
      onSelect: () => {},
      onClose: () => {}
    });
    expect(searchEmojis).toHaveBeenCalledExactlyOnceWith('he', Infinity);
    expect(container.querySelectorAll('button')).toHaveLength(state.emoji!.results.length);

    fakeEditor.setText(':zzzznothing');
    state.update();
    expect(state.emoji).toBeNull();
  });

  it('adds a trailing space when inserting a choice from any picker', () => {
    const fakeEditor = editor('See #gen! later', 8);
    const state = new AutocompleteState(() => fakeEditor.api, { rooms: true });
    state.update();
    state.selectRoom('https://chat.example/room');
    expect(fakeEditor.getText()).toBe('See https://chat.example/room ! later');
    expect(state.room).toBeNull();
    fakeEditor.setText('@ali');
    state.update();
    state.selectMention('alice');
    expect(fakeEditor.getText()).toBe('@alice ');
    expect(state.mention).toBeNull();
    fakeEditor.setText(':fi');
    state.update();
    state.selectEmoji('🔥');
    expect(fakeEditor.getText()).toBe('🔥 ');
    expect(state.emoji).toBeNull();
  });
  it('replaces only the room query and resets on dismissal and room changes', () => {
    const fakeEditor = editor('See #gen tomorrow', 8);
    const state = new AutocompleteState(() => fakeEditor.api, { rooms: true });
    state.update();
    expect(state.room).toEqual({ query: 'gen', triggerStart: 4 });
    state.selectRoom('https://chat.example/chat/-/R12345678901234');
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
      const state = new AutocompleteState(() => fakeEditor.api, { rooms: true });
      state.update();
      expect(state.room).toBeNull();
    }
  );

  it('keeps room completion disabled in other fields and in code blocks', () => {
    const fakeEditor = editor('#general');
    const disabled = new AutocompleteState(() => fakeEditor.api);
    disabled.update();
    expect(disabled.room).toBeNull();
    fakeEditor.api.isInCodeBlock = () => true;
    const state = new AutocompleteState(() => fakeEditor.api, { rooms: true });
    state.update();
    expect(state.room).toBeNull();
  });

  it.each(['(:he', '!:he', ':wave::he', '👋:he', ':heart: :he'])(
    'opens and completes emoji after a valid boundary: %s',
    (text) => {
      const fakeEditor = editor(text);
      const state = new AutocompleteState(() => fakeEditor.api);
      state.update();
      expect(state.emoji?.query).toBe('he');
      state.selectEmoji('❤️');
      expect(fakeEditor.getText()).toBe(text.slice(0, -3) + '❤️ ');
    }
  );

  it.each(['hello:he', '_:he', 'é:he', '中:he', '𐐀:he', 'e\u0301:he', ':he\n'])(
    'does not open emoji autocomplete inside words: %s',
    (text) => {
      const fakeEditor = editor(text);
      const state = new AutocompleteState(() => fakeEditor.api);
      state.update();
      expect(state.emoji).toBeNull();
    }
  );

  it.each([':+1', ':-1'])('supports signed emoji aliases: %s', (text) => {
    const fakeEditor = editor(text);
    const state = new AutocompleteState(() => fakeEditor.api);
    state.update();
    expect(state.emoji?.query).toBe(text.slice(1));
  });

  it('shows mention autocomplete when an @ partial has characters, even before members load', () => {
    const fakeEditor = editor('@');
    const state = new AutocompleteState(() => fakeEditor.api);

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
    const state = new AutocompleteState(() => fakeEditor.api);

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
    const state = new AutocompleteState(() => fakeEditor.api);

    state.update();
    expect(state.mention?.query).toBe('al');
    expect(state.emoji).toBeNull();

    fakeEditor.setText('@al :fi');
    state.update();

    expect(state.emoji?.query).toBe('fi');
    expect(state.mention).toBeNull();
  });

  it('completes a mention after leading text', () => {
    const fakeEditor = editor('Hey @ali');
    const state = new AutocompleteState(() => fakeEditor.api);

    state.update();
    state.selectMention('alice');
    expect(fakeEditor.getText()).toBe('Hey @alice ');
  });

  it.each(['here', 'helpdesk'])('inserts the selected virtual or role handle %s', (handle) => {
    const fakeEditor = editor('@he');
    const state = new AutocompleteState(() => fakeEditor.api);

    state.update();
    expect(state.mention?.query).toBe('he');

    state.selectMention(handle);
    expect(fakeEditor.getText()).toBe(`@${handle} `);
  });

  it('selects an emoji by replacing the shortcode before the cursor', () => {
    const fakeEditor = editor('hello :fi');
    const state = new AutocompleteState(() => fakeEditor.api);

    state.update();
    state.selectEmoji('🔥');

    expect(fakeEditor.getText()).toBe('hello 🔥 ');
    expect(state.emoji).toBeNull();
  });
});
