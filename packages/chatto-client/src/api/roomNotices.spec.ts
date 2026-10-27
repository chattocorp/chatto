import { beforeEach, describe, expect, it, vi } from 'vitest';
import { Code, ConnectError } from '@connectrpc/connect';
import { RoomService } from '@chatto/api-types/api/v1/rooms_connect';
import { createRoomNoticesAPI } from './roomNotices.js';
import { fakeServer, mockService, receivedRequest } from '../testing/fakeServer.js';

const mocks = mockService(RoomService);
const api = () => createRoomNoticesAPI(fakeServer((router) => router.service(RoomService, mocks)));

describe('room notice API', () => {
  beforeEach(() => vi.resetAllMocks());
  it('keeps correction and republish intent explicit and returns canonical versions', async () => {
    mocks.publishNotice.mockReturnValue({
      notice: { body: 'Corrected', version: 'v2', publicationId: 'p1' }
    });
    expect(await api().publish('R1', 'Corrected', 'v1', false)).toMatchObject({
      version: 'v2',
      publicationId: 'p1'
    });
    expect(receivedRequest(mocks.publishNotice)).toMatchObject({
      roomId: 'R1',
      body: 'Corrected',
      expectedVersion: 'v1',
      republish: false
    });
    mocks.publishNotice.mockClear();
    await api().publish('R1', 'Corrected', 'v2', true);
    expect(receivedRequest(mocks.publishNotice)).toMatchObject({
      republish: true,
      expectedVersion: 'v2'
    });
  });
  it('preserves clear versions and stale-write errors without retrying', async () => {
    mocks.clearNotice.mockReturnValue({ notice: { version: 'v3' } });
    expect(await api().clear('R1', 'v2')).toMatchObject({ body: '', version: 'v3' });
    mocks.publishNotice.mockImplementation(() => {
      throw new ConnectError('Changed', Code.Aborted);
    });
    await expect(api().publish('R1', 'draft', 'v2', false)).rejects.toMatchObject({
      code: Code.Aborted
    });
    expect(mocks.publishNotice).toHaveBeenCalledTimes(1);
  });
  it('cancels reads through the caller signal', async () => {
    await expect(api().get('R1', { signal: AbortSignal.abort() })).rejects.toMatchObject({
      code: Code.Canceled
    });
  });
});
