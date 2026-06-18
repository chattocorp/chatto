import { describe, expect, it } from 'vitest';
import { RoomType } from '$lib/gql/graphql';
import {
  roomIDFromURLSegment,
  roomMessagePathForSegment,
  roomPath,
  roomPathForSegment,
  roomThreadPathForSegment,
  roomURLSegment
} from './roomUrls';

describe('room URL helpers', () => {
  it('uses room ID plus channel name as canonical room URL segments', () => {
    expect(roomURLSegment({ id: 'R1', name: 'General', type: RoomType.Channel })).toBe(
      'R1-General'
    );
  });

  it('keeps ordinary capital-R channel names in the cosmetic suffix', () => {
    expect(roomURLSegment({ id: 'R1', name: 'Random', type: RoomType.Channel })).toBe('R1-Random');
  });

  it('uses DM IDs as canonical room URL segments', () => {
    expect(roomURLSegment({ id: 'deadbeef123456', name: '', type: RoomType.Dm })).toBe(
      'deadbeef123456'
    );
  });

  it('builds room paths from canonical targets and raw segments', () => {
    expect(roomPath('origin', { id: 'R1', name: 'General', type: RoomType.Channel })).toBe(
      '/chat/-/R1'
    );
    expect(roomPathForSegment('-', 'R1')).toBe('/chat/-/R1');
    expect(roomPathForSegment('-', 'R1-General')).toBe('/chat/-/R1-General');
    expect(roomThreadPathForSegment('-', 'R1', 'Eroot')).toBe('/chat/-/R1/Eroot');
    expect(roomMessagePathForSegment('-', 'R1', 'Emsg')).toBe('/chat/-/R1/m/Emsg');
  });

  it('extracts room IDs from canonical URL segments', () => {
    expect(roomIDFromURLSegment('R7gUDvZNyvHkk4K-general-chat')).toBe('R7gUDvZNyvHkk4K');
    expect(roomIDFromURLSegment('deadbeef123456')).toBe('deadbeef123456');
    expect(roomIDFromURLSegment('R1-General')).toBe('R1');
  });
});
