/** Collect a bounded channel summary and thread detail without giving the model posting tools. */
import type { ExtensionFactory } from '@earendil-works/pi-coding-agent';
import { Type } from 'typebox';

/** The host adds the source link to the summary and posts details under the returned root ID. */
export interface AnnouncementDraft {
  summary: string;
  details: string;
}

/** A terminal tool selects the two message bodies; intermediate model text is never delivered. */
export function announcementResponse(signal: AbortSignal) {
  let draft: AnnouncementDraft | undefined;
  const extension: ExtensionFactory = (pi) => {
    pi.on('tool_call', () =>
      draft !== undefined
        ? { block: true, reason: 'The announcement is complete.', terminate: true }
        : undefined
    );
    pi.registerTool({
      name: 'finish_announcement',
      label: 'Finish announcement',
      description:
        'Finish with a short channel summary and a separate Markdown thread reply. This prepares content only; the host handles posting. Call once, after research, without other tool calls.',
      parameters: Type.Object(
        {
          summary: Type.String({
            minLength: 1,
            maxLength: 400,
            description:
              'Brief Markdown for the channel, up to 400 characters including formatting. Follow the configured prompt; put detailed explanation in the thread.'
          }),
          details: Type.String({
            minLength: 1,
            maxLength: 4000,
            description:
              'Useful Markdown detail for the thread, including evidence limitations when needed.'
          })
        },
        { additionalProperties: false }
      ),
      async execute(_id, value) {
        signal.throwIfAborted();
        if (draft !== undefined) throw new Error('Announcement already completed.');
        const summary = value.summary.trim(),
          details = value.details.trim();
        if (!summary || summary.length > 400 || !details || details.length > 4000)
          throw new Error(
            'Provide a short Markdown summary up to 400 characters and details up to 4000 characters.'
          );
        draft = { summary, details };
        return {
          content: [
            {
              type: 'text' as const,
              text: 'Announcement prepared. Delivery is handled by the host.'
            }
          ],
          details: {},
          terminate: true
        };
      }
    });
  };
  return {
    extension,
    result(): AnnouncementDraft {
      signal.throwIfAborted();
      if (draft === undefined) throw new Error('Agent did not finish the announcement.');
      return draft;
    }
  };
}
