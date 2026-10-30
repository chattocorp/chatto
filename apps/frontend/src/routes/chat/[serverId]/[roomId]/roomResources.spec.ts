import { describe, expect, it, vi } from 'vitest';
import { RoomResources } from './roomResources';

function resource() {
  const release = vi.fn();
  return { retain: vi.fn(() => release), release };
}

describe('RoomResources', () => {
  it('keeps pins and notices retained when the Files panel opens and closes', () => {
    const owner = new RoomResources();
    const files = resource();
    const pins = resource();
    const notice = resource();
    owner.update({ files: null, pins, notice }, null);
    owner.update({ files, pins, notice }, null);
    owner.update({ files, pins, notice }, null);
    owner.update({ files: null, pins, notice }, null);
    expect(files.retain).toHaveBeenCalledOnce();
    expect(files.release).toHaveBeenCalledOnce();
    for (const retained of [pins, notice]) {
      expect(retained.retain).toHaveBeenCalledOnce();
      expect(retained.release).not.toHaveBeenCalled();
    }
    owner.dispose();
    owner.dispose();
    expect(pins.release).toHaveBeenCalledOnce();
    expect(notice.release).toHaveBeenCalledOnce();
  });

  it('releases all old-room resources before acquiring the next room', () => {
    const owner = new RoomResources();
    const old = { files: resource(), pins: resource(), notice: resource() };
    const next = { files: resource(), pins: resource(), notice: resource() };
    owner.update(old, null);
    owner.update(next, null);
    for (const previous of Object.values(old)) {
      expect(previous.release).toHaveBeenCalledOnce();
      for (const current of Object.values(next)) {
        expect(previous.release.mock.invocationCallOrder[0]).toBeLessThan(
          current.retain.mock.invocationCallOrder[0]
        );
      }
    }
    owner.dispose();
    for (const current of Object.values(next)) expect(current.release).toHaveBeenCalledOnce();
  });

  it('releases revoked resources and reacquires them when access returns', () => {
    const owner = new RoomResources();
    const pins = resource();
    const notice = resource();
    owner.update({ files: null, pins, notice }, null);
    owner.update({ files: null, pins: null, notice }, null);
    expect(pins.release).toHaveBeenCalledOnce();
    expect(notice.release).not.toHaveBeenCalled();
    owner.update({ files: null, pins, notice }, null);
    expect(pins.retain).toHaveBeenCalledTimes(2);
    expect(notice.retain).toHaveBeenCalledOnce();
    owner.dispose();
  });

  it('reloads reset members without restarting other resources', () => {
    const owner = new RoomResources();
    const pins = resource();
    const members = { ensureLoaded: vi.fn() };
    const selection = { files: null, pins, notice: null };
    owner.update(selection, members);
    owner.update(selection, null);
    owner.update(selection, members);
    expect(members.ensureLoaded).toHaveBeenCalledTimes(2);
    expect(pins.retain).toHaveBeenCalledOnce();
    expect(pins.release).not.toHaveBeenCalled();
    owner.dispose();
  });
});
