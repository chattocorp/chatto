/** Botto is a Runling project. The CLI owns environment loading, serving, and shutdown. */
import { join } from 'node:path';
import { defineWebConfig } from 'runling/web';
import { conversation } from './src/conversation.ts';
import { readSettings } from './src/settings.ts';
import { createGithubAnnouncements } from './src/announcements.ts';
import { runSource } from './src/source.ts';

process.umask(0o077);
const settings = readSettings(process.env.BOTTO_CONFIG);
process.env.RUNLING_DATA_DIRECTORY ??= join(settings.directory, 'runling');

const announcements = settings.announcements ? createGithubAnnouncements(settings) : undefined;

export default defineWebConfig({
  workflows: {
    bottoConversation: conversation,
    ...(announcements ? { githubAnnouncement: announcements.workflow } : {})
  },
  sources: {
    chatto: (ctx) => runSource(settings, ctx),
    ...(announcements ? { githubAnnouncements: announcements.source } : {})
  }
});
