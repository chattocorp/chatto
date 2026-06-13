import { describe, it, expect } from 'vitest';
import { PresenceStatus } from '$lib/gql/graphql';
import type { RoomMember } from '$lib/state/room';
import type { TipTapEditorApi } from './TipTapEditor.svelte';
import { AutocompleteState } from './autocomplete.svelte';

function member(login: string, displayName = login): RoomMember {
  return {
    id: `user_${login}`,
    login,
    displayName,
    avatarUrl: null,
    presenceStatus: PresenceStatus.Offline
  };
}

function editor(initialText: string): {
  api: TipTapEditorApi;
  getText: () => string;
  setText: (text: string) => void;
} {
  let text = initialText;
  let cursor = text.length;
  return {
    api: {
      getText: () => text,
      setContent: (next) => {
        text = next;
        cursor = text.length;
      },
      focus: () => {},
      getTextBeforeCursor: () => text.slice(0, cursor),
      replaceTextBeforeCursor: (charCount, replacement) => {
        text = text.slice(0, cursor - charCount) + replacement + text.slice(cursor);
        cursor = cursor - charCount + replacement.length;
      }
    },
    getText: () => text,
    setText: (next) => {
      text = next;
      cursor = text.length;
    }
  };
}

function tabEvent(): KeyboardEvent {
  return new KeyboardEvent('keydown', { key: 'Tab', cancelable: true });
}

describe('AutocompleteState', () => {
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
    expect(fakeEditor.getText()).toBe('@alice ');

    expect(state.handleTabCompletion(tabEvent())).toBe(true);
    expect(fakeEditor.getText()).toBe('@alicia ');

    state.resetTabCompletion();
    expect(state.handleTabCompletion(tabEvent())).toBe(false);
    expect(fakeEditor.getText()).toBe('@alicia ');
  });

  it('selects a mention from the popup and seeds Tab cycling only for Tab selection', () => {
    const fakeEditor = editor('@ali');
    const state = new AutocompleteState(
      () => fakeEditor.api,
      () => [member('alice'), member('alicia')],
      () => []
    );

    state.update();
    state.selectMention('alice', false);

    expect(fakeEditor.getText()).toBe('@alice ');
    expect(state.tabCompletion).toBeNull();

    fakeEditor.setText('@ali');
    state.update();
    state.selectMention('alice', true);

    expect(state.tabCompletion?.candidates).toEqual(['alice', 'alicia']);
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
    expect(fakeEditor.getText()).toBe('@helena ');

    expect(state.handleTabCompletion(tabEvent())).toBe(true);
    expect(fakeEditor.getText()).toBe('@here ');

    expect(state.handleTabCompletion(tabEvent())).toBe(true);
    expect(fakeEditor.getText()).toBe('@helpdesk ');
  });

  it('selects an emoji by replacing the shortcode before the cursor', () => {
    const fakeEditor = editor('hello :fi');
    const state = new AutocompleteState(
      () => fakeEditor.api,
      () => []
    );

    state.update();
    state.selectEmoji('🔥');

    expect(fakeEditor.getText()).toBe('hello 🔥 ');
    expect(state.emoji).toBeNull();
  });
});
