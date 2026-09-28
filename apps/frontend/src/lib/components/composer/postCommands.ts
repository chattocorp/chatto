const shrug = '¯\\_(ツ)_/¯';

/** Expand a leading post command after the draft has been normalized for sending. */
export function expandPostCommand(body: string): string {
  const match = /^\/shrug(?=$|\s)/u.exec(body);
  if (!match) return body;

  const message = body.slice(match[0].length).trim();
  return message ? `${message} ${shrug}` : shrug;
}
