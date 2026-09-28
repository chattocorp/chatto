/** Detect the language of the people in a conversation, so the supervisor replies in it. The
 * supervisor model has switched languages at random in announcements; the host checks them. */
import { francAll } from 'franc-min';

/** Scripts that a text can mostly use. Latin covers most European languages. */
const SCRIPTS = [
  'Latin',
  'Cyrillic',
  'Greek',
  'Hebrew',
  'Arabic',
  'Han',
  'Hiragana',
  'Katakana',
  'Hangul',
  'Devanagari',
  'Thai'
] as const;
const scriptPatterns = SCRIPTS.map((script) => ({
  script,
  pattern: new RegExp(`\\p{Script=${script}}`, 'gu')
}));

/** The script that most letters of `text` use, or undefined for text without letters. */
export function dominantScript(text: string): string | undefined {
  let best: { script: string; count: number } | undefined;
  for (const { script, pattern } of scriptPatterns) {
    const count = text.match(pattern)?.length ?? 0;
    if (count && (!best || count > best.count)) best = { script, count };
  }
  return best?.script;
}

/** Remove mentions, code, and links, which do not show a language. */
const prose = (text: string) =>
  text
    .replace(/```[\s\S]*?```|`[^`]*`/g, ' ')
    .replace(/https?:\/\/\S+|@\S+/g, ' ')
    .trim();

/** Below this score for the people's language, an announcement is in another language. Real
 * announcements score 0.94 or more, even when a short one is misdetected; other languages
 * score about 0.7. */
const SAME_LANGUAGE_SCORE = 0.8;

/** The people's language, detected from their messages, with its English name. Undefined when
 * the messages are too short to tell. */
export interface ConversationLanguage {
  /** ISO 639-3 code, such as `eng`. */
  code: string;
  /** English name, such as `English`. */
  name: string;
  script: string;
}

/** Detect the language of the people's messages in a conversation. */
export function conversationLanguage(
  messages: readonly string[]
): ConversationLanguage | undefined {
  const text = messages.map(prose).join('\n');
  const script = dominantScript(text);
  if (!script || text.length < 40) return undefined;
  const [top] = francAll(text, { minLength: 40 });
  if (!top || top[0] === 'und') return undefined;
  let name: string | undefined;
  try {
    name = new Intl.DisplayNames(['en'], { type: 'language' }).of(top[0]);
  } catch {
    return undefined;
  }
  return name && name !== top[0] ? { code: top[0], name, script } : undefined;
}

/** True when `text` is clearly in another language or script than the conversation. */
export function differentLanguage(language: ConversationLanguage, text: string): boolean {
  const cleaned = prose(text);
  const script = dominantScript(cleaned);
  if (!script) return false;
  if (script !== language.script) return true;
  const scores = francAll(cleaned, { minLength: 10 });
  if (!scores.length || scores[0]![0] === 'und') return false;
  const score = scores.find(([code]) => code === language.code)?.[1] ?? 0;
  return score < SAME_LANGUAGE_SCORE;
}
