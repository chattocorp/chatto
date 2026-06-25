import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { userEvent } from 'vitest/browser';
import { q } from '$lib/test-utils';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    mutation: vi.fn(),
    onroomcreated: vi.fn()
  }
}));

vi.mock('$lib/state/server/connection.svelte', () => ({
  useConnection: () => () => ({
    client: {
      mutation: mocks.mutation
    }
  })
}));

import CreateRoom from './CreateRoom.svelte';

function operationName(document: unknown): string | undefined {
  return (
    document as {
      definitions?: Array<{ name?: { value?: string } }>;
    }
  ).definitions?.[0]?.name?.value;
}

async function fillNameAndSubmit(container: HTMLElement, name = 'general'): Promise<void> {
  await userEvent.type(q(container, '#room-name') as HTMLInputElement, name);
  (q(container, 'button[type="submit"]') as HTMLButtonElement).click();
}

beforeEach(() => {
  vi.clearAllMocks();
  mocks.mutation.mockImplementation((document: unknown) => {
    const op = operationName(document);
    return {
      toPromise: vi.fn().mockResolvedValue({
        data:
          op === 'CreateRoom'
            ? { createRoom: { id: 'room-1', name: 'general', description: null } }
            : { joinRoom: { id: 'room-1' } },
        error: null
      })
    };
  });
});

describe('CreateRoom', () => {
  it('omits isUniversal for normal room creation so older servers accept the input', async () => {
    const { container } = render(CreateRoom, {
      groupId: 'group-1',
      onroomcreated: mocks.onroomcreated
    });

    await fillNameAndSubmit(container);

    await vi.waitFor(() => {
      expect(mocks.onroomcreated).toHaveBeenCalledWith('room-1');
    });
    expect(mocks.mutation.mock.calls[0][1]).toEqual({
      input: {
        name: 'general',
        description: undefined,
        groupId: 'group-1'
      }
    });
  });

  it('falls back without isUniversal when an older server rejects the new input field', async () => {
    mocks.mutation
      .mockImplementationOnce(() => ({
        toPromise: vi.fn().mockResolvedValue({
          data: null,
          error: {
            graphQLErrors: [
              { message: 'Field "isUniversal" is not defined by type "CreateRoomInput".' }
            ],
            message: 'Field "isUniversal" is not defined by type "CreateRoomInput".'
          }
        })
      }))
      .mockImplementationOnce(() => ({
        toPromise: vi.fn().mockResolvedValue({
          data: { createRoom: { id: 'room-1', name: 'general', description: null } },
          error: null
        })
      }))
      .mockImplementationOnce(() => ({
        toPromise: vi.fn().mockResolvedValue({
          data: { joinRoom: { id: 'room-1' } },
          error: null
        })
      }));

    const { container } = render(CreateRoom, {
      groupId: 'group-1',
      onroomcreated: mocks.onroomcreated
    });

    (q(container, '#room-universal') as HTMLInputElement).click();
    await fillNameAndSubmit(container);

    await vi.waitFor(() => {
      expect(mocks.onroomcreated).toHaveBeenCalledWith('room-1');
    });
    expect(mocks.mutation.mock.calls[0][1]).toEqual({
      input: {
        name: 'general',
        description: undefined,
        groupId: 'group-1',
        isUniversal: true
      }
    });
    expect(mocks.mutation.mock.calls[1][1]).toEqual({
      input: {
        name: 'general',
        description: undefined,
        groupId: 'group-1'
      }
    });
  });
});
