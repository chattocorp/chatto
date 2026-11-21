import { describe, expect, it } from 'vitest';
import { User } from '@chatto/api-types/api/v1/users_pb';
import { DirectoryMember } from '@chatto/api-types/api/v1/member_directory_pb';
import {
  applyPresentableDisplayNames,
  presentableDisplayNamesInterceptor
} from './presentableUsers.js';

const NANDI = '\u{1FB15}\u{1FB22}▌\u{1FB1C}';

const member = (displayName: string, login: string) =>
  new DirectoryMember({ user: new User({ id: login, login, displayName }) });

describe('applyPresentableDisplayNames', () => {
  it('replaces invalid names in nested users, arrays, and maps', () => {
    const value = {
      members: [member(NANDI, 'nandi'), member('Alice', 'alice')],
      users: { u1: new User({ login: 'bob', displayName: '██' }) },
      oneof: { case: 'user', value: new User({ login: 'carol', displayName: NANDI }) }
    };
    applyPresentableDisplayNames(value);
    expect(value.members.map((m) => m.user?.displayName)).toEqual(['nandi', 'Alice']);
    expect(value.users.u1.displayName).toBe('bob');
    expect(value.oneof.value.displayName).toBe('carol');
  });
});

describe('presentableDisplayNamesInterceptor', () => {
  it('fixes unary responses', async () => {
    const response = { stream: false, message: member(NANDI, 'nandi') };
    const result = await presentableDisplayNamesInterceptor(async () => response as never)(
      {} as never
    );
    expect((result.message as DirectoryMember).user?.displayName).toBe('nandi');
  });

  it('fixes each stream message', async () => {
    async function* messages() {
      yield member(NANDI, 'nandi');
      yield member('Alice', 'alice');
    }
    const result = await presentableDisplayNamesInterceptor(
      async () => ({ stream: true, message: messages() }) as never
    )({} as never);
    const names: string[] = [];
    for await (const message of result.message as AsyncIterable<DirectoryMember>) {
      names.push(message.user?.displayName ?? '');
    }
    expect(names).toEqual(['nandi', 'Alice']);
  });
});
