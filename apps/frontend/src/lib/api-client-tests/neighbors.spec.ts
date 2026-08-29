import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createNeighborAPI } from '$lib/api-client/neighbors';

const mocks = vi.hoisted(() => ({
  createClient: vi.fn(),
  createConnectTransport: vi.fn(),
  listNeighbors: vi.fn(),
  createNeighbor: vi.fn(),
  updateNeighbor: vi.fn(),
  deleteNeighbor: vi.fn()
}));

vi.mock('@connectrpc/connect', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@connectrpc/connect')>();
  return { ...actual, createClient: mocks.createClient };
});

vi.mock('@connectrpc/connect-web', () => ({
  createConnectTransport: mocks.createConnectTransport
}));

describe('createNeighborAPI', () => {
  beforeEach(() => {
    for (const mock of Object.values(mocks)) mock.mockReset();
    mocks.createConnectTransport.mockReturnValue({ kind: 'transport' });
    mocks.createClient.mockReturnValue({
      listNeighbors: mocks.listNeighbors,
      createNeighbor: mocks.createNeighbor,
      updateNeighbor: mocks.updateNeighbor,
      deleteNeighbor: mocks.deleteNeighbor
    });
  });

  it('maps CRUD requests and opaque revisions', async () => {
    const api = createNeighborAPI({ baseUrl: 'https://local.example', bearerToken: 'token' });
    const first = {
      id: 'N1',
      origin: 'https://one.example',
      testimonial: 'A kind place.',
      revision: 'E1'
    };
    const second = {
      ...first,
      origin: 'https://two.example',
      testimonial: null,
      revision: 'E2'
    };
    mocks.listNeighbors.mockResolvedValue({ neighbors: [first] });
    mocks.createNeighbor.mockResolvedValue({ neighbor: first });
    mocks.updateNeighbor.mockResolvedValue({ neighbor: second });
    mocks.deleteNeighbor.mockResolvedValue({});

    await expect(api.list()).resolves.toEqual([first]);
    await expect(api.create(first.origin, first.testimonial)).resolves.toEqual(first);
    await expect(api.update(first, second.origin, '')).resolves.toEqual(second);
    await expect(api.delete(second)).resolves.toBeUndefined();

    expect(mocks.updateNeighbor).toHaveBeenCalledWith(
      { neighborId: 'N1', origin: 'https://two.example', testimonial: '', revision: 'E1' },
      { headers: { Authorization: 'Bearer token' } }
    );
    expect(mocks.deleteNeighbor).toHaveBeenCalledWith(
      { neighborId: 'N1', revision: 'E2' },
      { headers: { Authorization: 'Bearer token' } }
    );
  });
});
