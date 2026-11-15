import { withHelpServer } from './links';
import { m } from '$lib/i18n/messages';

/** Stable article IDs also define the public help URLs. */
export const helpTopics = ['conversations', 'formatting', 'bots', 'phone'] as const;
export type HelpTopic = (typeof helpTopics)[number];

/** Shared topic symbols for navigation and compact card artwork. */
export const helpTopicIcons: Record<HelpTopic, string> = {
  conversations: 'iconify icon-[uil--comments]',
  formatting: 'iconify icon-[uil--edit]',
  phone: 'iconify icon-[uil--mobile-android]',
  bots: 'iconify icon-[uil--robot]'
};

/** Derive translated navigation and overview cards from the same article list. */
export function helpGroups(serverSegment?: string | null) {
  const groups: { audience: 'users' | 'admins'; topics: HelpTopic[] }[] = [
    { audience: 'users', topics: ['conversations', 'formatting', 'phone'] },
    { audience: 'admins', topics: ['bots'] }
  ];
  return groups.map(({ audience, topics }) => ({
    label: m(`help.${audience}`),
    persistKey: `collapsible:help:${audience}`,
    items: topics.map((topic) => ({
      topic,
      href: withHelpServer(`/help/${topic}`, serverSegment),
      label: m(`help.${topic}.title`),
      description: m(`help.${topic}.description`),
      icon: helpTopicIcons[topic]
    }))
  }));
}
