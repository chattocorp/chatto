/** Local fixtures for renderer experiments. No account, network, or disk state. */
export type RoomId = 'general' | 'rendering' | 'playground';

export interface DemoRoom {
  id: RoomId;
  name: string;
  description: string;
}

export interface DemoMessage {
  id: string;
  author: string;
  time: string;
  body: string;
  /** Only repository-owned sample content enters the native Markdown renderer. */
  format: 'markdown' | 'text';
}

export const rooms: readonly DemoRoom[] = [
  { id: 'general', name: 'general', description: 'A fresh native experiment, built with Svelte.' },
  {
    id: 'rendering',
    name: 'rendering',
    description: 'Markdown, long messages, and a little room to scroll.'
  },
  {
    id: 'playground',
    name: 'playground',
    description: 'Try the composer. Everything here stays local.'
  }
];

export const maxMessages = 2_000;

/** Generate stable IDs and varied row heights to exercise native virtual scrolling. */
export function sampleMessages(room: RoomId, count: number, offset = 0): DemoMessage[] {
  return Array.from({ length: count }, (_, index) => {
    const number = offset + index + 1;
    const bodies = [
      `Sample ${number}: **Svelte state, native pixels.** A small message for a long conversation.`,
      `Sample ${number}: a paragraph with *emphasis*, **bold text**, and \`inline code\`.\n\nA second paragraph gives this row a different height.`,
      `Sample ${number}: a few things to try:\n\n- Switch channels\n- Select some text\n- Add more messages and scroll`,
      `Sample ${number}: a tiny code block.\n\n\`\`\`typescript\nlet count = $state(0);\nconst add = () => count++;\n\`\`\``,
      `Sample ${number}: café, 世界, and 🦀.\n\n> A conversation should feel at home on the desktop.`
    ];
    return {
      id: `${room}:sample:${number}`,
      author: number % 3 === 0 ? 'Native lab' : 'Chatto',
      time: `10:${String(number % 60).padStart(2, '0')}`,
      body: bodies[index % bodies.length]!,
      format: 'markdown'
    };
  });
}

/** A fresh independent history for each channel, with no data copied from a server. */
export function initialHistories(): Record<RoomId, DemoMessage[]> {
  return {
    general: sampleMessages('general', 80),
    rendering: sampleMessages('rendering', 36),
    playground: sampleMessages('playground', 8)
  };
}
