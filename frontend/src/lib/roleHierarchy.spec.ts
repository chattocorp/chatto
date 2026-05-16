import { describe, expect, it } from 'vitest';
import { bestPosition, viewerOutranksTarget, type RankRole } from './roleHierarchy';

// Mirrors the backend constants in cli/internal/core/rbac/engine.go.
// Higher position = higher rank.
const SYSTEM_ROLES: RankRole[] = [
  { name: 'everyone', position: 0 },
  { name: 'moderator', position: 100 },
  { name: 'admin', position: 900 },
  { name: 'owner', position: 1000 }
];

describe('bestPosition', () => {
  it('returns the highest position among the named roles', () => {
    expect(bestPosition(['moderator', 'admin'], SYSTEM_ROLES)).toBe(900);
  });

  it('returns -Infinity for an empty role list (unranked user)', () => {
    expect(bestPosition([], SYSTEM_ROLES)).toBe(-Infinity);
  });

  it('ignores unknown role names', () => {
    expect(bestPosition(['ghost', 'moderator'], SYSTEM_ROLES)).toBe(100);
  });

  it('handles custom roles at their explicit position', () => {
    const roles: RankRole[] = [...SYSTEM_ROLES, { name: 'self-hosters', position: 50 }];
    expect(bestPosition(['self-hosters', 'everyone'], roles)).toBe(50);
    expect(bestPosition(['self-hosters'], roles)).toBe(50);
  });
});

describe('viewerOutranksTarget', () => {
  it('owner outranks a moderator (the bug we filled)', () => {
    expect(viewerOutranksTarget(['owner'], ['moderator'], SYSTEM_ROLES)).toBe(true);
  });

  it('owner outranks an admin', () => {
    expect(viewerOutranksTarget(['owner'], ['admin'], SYSTEM_ROLES)).toBe(true);
  });

  it('admin outranks a moderator', () => {
    expect(viewerOutranksTarget(['admin'], ['moderator'], SYSTEM_ROLES)).toBe(true);
  });

  it('moderator does NOT outrank an admin', () => {
    expect(viewerOutranksTarget(['moderator'], ['admin'], SYSTEM_ROLES)).toBe(false);
  });

  it('peer admins do not outrank each other (strict outrank)', () => {
    expect(viewerOutranksTarget(['admin'], ['admin'], SYSTEM_ROLES)).toBe(false);
  });

  it('peer owners do not outrank each other', () => {
    expect(viewerOutranksTarget(['owner'], ['owner'], SYSTEM_ROLES)).toBe(false);
  });

  it('uses the highest role on each side, not the lowest', () => {
    // Target's mix doesn't matter — their *best* role is what counts.
    expect(viewerOutranksTarget(['admin'], ['everyone', 'moderator'], SYSTEM_ROLES)).toBe(true);
    // Viewer's mix likewise: presence of `everyone` doesn't drag them down.
    expect(viewerOutranksTarget(['everyone', 'admin'], ['moderator'], SYSTEM_ROLES)).toBe(true);
  });

  it('treats an unranked target as outrankable by any holder of a real role', () => {
    expect(viewerOutranksTarget(['moderator'], [], SYSTEM_ROLES)).toBe(true);
    expect(viewerOutranksTarget(['everyone'], [], SYSTEM_ROLES)).toBe(true);
  });

  it('returns false when neither side holds any role', () => {
    expect(viewerOutranksTarget([], [], SYSTEM_ROLES)).toBe(false);
  });

  it('a custom role positioned below moderator does not outrank moderator', () => {
    const roles: RankRole[] = [...SYSTEM_ROLES, { name: 'self-hosters', position: 50 }];
    expect(viewerOutranksTarget(['self-hosters'], ['moderator'], roles)).toBe(false);
  });

  it('a custom role positioned above moderator outranks moderator', () => {
    const roles: RankRole[] = [...SYSTEM_ROLES, { name: 'lead', position: 500 }];
    expect(viewerOutranksTarget(['lead'], ['moderator'], roles)).toBe(true);
  });
});
