import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createNeighborAPI } from '$lib/api-client/neighbors';
import { AdminServerService } from '@chatto/api-types/admin/v1/server_connect';
import { fakeServer, mockService, receivedRequest } from '$lib/test-utils';

const mocks = mockService(AdminServerService);

describe('createNeighborAPI', () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  it('maps CRUD requests and opaque revisions', async () => {
    const api = createNeighborAPI(
      fakeServer((router) => router.service(AdminServerService, mocks))
    );
    const first = {
      id: 'N1',
      origin: 'https://one.example',
      revision: 'E1'
    };
    const second = {
      ...first,
      origin: 'https://two.example',
      revision: 'E2'
    };
    mocks.listNeighbors.mockReturnValue({ neighbors: [first] });
    mocks.createNeighbor.mockReturnValue({ neighbor: first });
    mocks.updateNeighbor.mockReturnValue({ neighbor: second });
    mocks.deleteNeighbor.mockReturnValue({});

    await expect(api.list()).resolves.toEqual([first]);
    await expect(api.create(first.origin)).resolves.toEqual(first);
    await expect(api.update(first, second.origin)).resolves.toEqual(second);
    await expect(api.delete(second)).resolves.toBeUndefined();

    expect(receivedRequest(mocks.updateNeighbor)).toMatchObject({
      neighborId: 'N1',
      origin: 'https://two.example',
      revision: 'E1',
      updateMask: { paths: ['origin'] }
    });
    expect(receivedRequest(mocks.deleteNeighbor)).toMatchObject({
      neighborId: 'N1',
      revision: 'E2'
    });
  });
});
