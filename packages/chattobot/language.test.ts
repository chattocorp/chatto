import { expect, test } from 'vitest';
import { conversationLanguage, differentLanguage, dominantScript } from './language.ts';

const english = [
  'Bug: when I receive a reply in a thread I am following, I get two notifications.',
  '@chatto_bot what do you think?',
  'I had to restart you. Can you pick up where you left off?'
];

test('the conversation language comes from the people’s messages', () => {
  expect(conversationLanguage(english)).toEqual({ code: 'eng', name: 'English', script: 'Latin' });
  expect(
    conversationLanguage([
      'Ich habe gerade zwei Benachrichtigungen für dieselbe Antwort bekommen.',
      'Kannst du dir das bitte ansehen und einen Plan machen?'
    ])
  ).toMatchObject({ code: 'deu', name: 'German' });
  expect(conversationLanguage(['Boo!'])).toBeUndefined();
  expect(dominantScript('Hello `/shrug` שלום')).toBe('Latin');
});

test('announcements in another language or script are detected; short English ones are not', () => {
  const language = conversationLanguage(english)!;
  // Real announcements from runs in which the model switched languages.
  expect(
    differentLanguage(
      language,
      'J’ouvre le chantier interrompu exactement là où il s’est arrêté et je poursuis l’implémentation avec les décisions déjà convenues.'
    )
  ).toBe(true);
  expect(
    differentLanguage(
      language,
      'אני אבדוק כיצד נבנים מוני ההתראות בסרגל החדרים ואמליץ על תיקון מתאים.'
    )
  ).toBe(true);
  for (const announcement of [
    'I’ll resume the interrupted implementation where it stopped.',
    'Can you give me an implementation plan first please?',
    'I’ll implement `/shrug` in `MessageComposer.svelte` and open a PR.'
  ])
    expect(differentLanguage(language, announcement)).toBe(false);
});
