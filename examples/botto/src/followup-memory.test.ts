/** Exercise durable duplicate prevention and uncertain-send handling without model cooperation. */
import { expect, test, vi } from 'vitest';
import { deliverFollowup, type MeansCall } from './followup-memory.ts';

const scope = { repository: 'owner/repo', serverUrl: 'https://chat.example.com' };
const input = {
  originId: 'origin',
  roomId: 'room',
  threadId: 'thread',
  commitSha: 'a'.repeat(40),
  body: 'Fix landed.'
};
function setup() {
  const origin = {
    repository: scope.repository,
    server_url: scope.serverUrl,
    issue_number: 123,
    room_id: 'room',
    thread_id: 'thread'
  };
  type Doc = { id: string; hash: string; frontmatter: { data: Record<string, unknown> } };
  const docs: Record<string, Doc> = {
    origin: { id: 'origin', hash: '0', frontmatter: { data: { issue_origin: origin } } }
  };
  const matches = (value: unknown, filter: unknown): boolean => {
    if (filter && typeof filter === 'object')
      return (
        !!value &&
        typeof value === 'object' &&
        Object.entries(filter).every(([key, entry]) =>
          matches((value as Record<string, unknown>)[key], entry)
        )
      );
    return value === filter;
  };
  const call = vi.fn<MeansCall>(async (name, args) => {
    if (name === 'search')
      return {
        documents: structuredClone(
          Object.values(docs).filter((doc) => matches(doc.frontmatter.data, args.filter))
        ),
        diagnostics: [],
        has_more: false
      };
    const doc = docs[String(args.id)];
    if (!doc) throw new Error('Missing record');
    if (name === 'read') return structuredClone(doc);
    if (name === 'update') {
      if (args.hash !== doc.hash) throw new Error('Hash conflict');
      Object.assign(doc.frontmatter.data, (args.frontmatter as { data: object }).data);
      doc.hash = String(Number(doc.hash) + 1);
      return structuredClone(doc);
    }
    throw new Error('Unexpected tool');
  });
  const send = vi.fn(async () => 'posted');
  return { docs, call, send, run: () => deliverFollowup(call, scope, input, send) };
}

test('records delivery on the origin and skips later calls without another send', async () => {
  const { run, docs, send } = setup();
  expect(await run()).toEqual({ messageId: 'posted', alreadyDelivered: false });
  expect(docs.origin!.frontmatter.data.delivery).toMatchObject({
    status: 'delivered',
    message_id: 'posted'
  });
  expect(await run()).toEqual({ messageId: 'posted', alreadyDelivered: true });
  expect(send).toHaveBeenCalledTimes(1);
});

test('legacy receipts suppress sending even when the agent selects a duplicate origin', async () => {
  const { docs, run, send } = setup();
  docs.alias = { ...structuredClone(docs.origin!), id: 'alias' };
  docs.receipt = {
    id: 'receipt',
    hash: '0',
    frontmatter: {
      data: {
        issue_notification: {
          origin_id: 'alias',
          message_id: 'old-post',
          commit_sha: input.commitSha
        }
      }
    }
  };
  expect(await run()).toEqual({ messageId: 'old-post', alreadyDelivered: true });
  expect(send).not.toHaveBeenCalled();
});

test('failed or incomplete lookups never send', async () => {
  for (const result of [
    { documents: [], diagnostics: ['bad'], has_more: false },
    { documents: [], diagnostics: [], has_more: true },
    { documents: [], diagnostics: [], has_more: false }
  ]) {
    const { call, send } = setup();
    const lookup: MeansCall = (name, args) =>
      name === 'search' ? Promise.resolve(result) : call(name, args);
    await expect(deliverFollowup(lookup, scope, input, send)).rejects.toThrow();
    expect(send).not.toHaveBeenCalled();
  }
});

test('wrong destinations and repository scope never send', async () => {
  const { call, send } = setup();
  for (const override of [{ roomId: 'wrong' }, { threadId: 'wrong' }])
    await expect(deliverFollowup(call, scope, { ...input, ...override }, send)).rejects.toThrow(
      'does not match'
    );
  await expect(
    deliverFollowup(call, { ...scope, repository: 'other/repo' }, input, send)
  ).rejects.toThrow('does not match');
  expect(send).not.toHaveBeenCalled();
});

test('an uncertain send remains held across later attempts', async () => {
  const { run, send, docs } = setup();
  send.mockRejectedValueOnce(new Error('Disconnected after send'));
  await expect(run()).rejects.toThrow('Disconnected');
  expect(docs.origin!.frontmatter.data.delivery).toMatchObject({ status: 'sending' });
  await expect(run()).rejects.toThrow('operator review');
  expect(send).toHaveBeenCalledTimes(1);
});

test('receipt write failure leaves durable intent and blocks a later resend', async () => {
  const { call, send } = setup();
  let writes = 0;
  const failReceipt: MeansCall = (name, args) => {
    if (name === 'update' && ++writes === 2) throw new Error('Receipt save failed');
    return call(name, args);
  };
  await expect(deliverFollowup(failReceipt, scope, input, send)).rejects.toThrow(
    'Receipt save failed'
  );
  await expect(deliverFollowup(call, scope, input, send)).rejects.toThrow('operator review');
  expect(send).toHaveBeenCalledTimes(1);
});

test('intent write failure prevents the external write', async () => {
  const { call, send } = setup();
  const failIntent: MeansCall = (name, args) => {
    if (name === 'update') throw new Error('Hash conflict');
    return call(name, args);
  };
  await expect(deliverFollowup(failIntent, scope, input, send)).rejects.toThrow('Hash conflict');
  expect(send).not.toHaveBeenCalled();
});
