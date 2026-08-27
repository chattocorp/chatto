import { describe, expect, it } from 'vitest';
import { isCurrentResourceOperation } from './resourceOperationFence';

describe('isCurrentResourceOperation', () => {
  const target = { resourceId: 'room-a', generation: 4 };

  it('accepts a response for the same resource and load generation', () => {
    expect(isCurrentResourceOperation(target, 'room-a', 4)).toBe(true);
  });

  it('rejects a response after navigation changes the resource', () => {
    expect(isCurrentResourceOperation(target, 'room-b', 4)).toBe(false);
  });

  it('rejects a response after the resource has reloaded', () => {
    expect(isCurrentResourceOperation(target, 'room-a', 5)).toBe(false);
  });
});
