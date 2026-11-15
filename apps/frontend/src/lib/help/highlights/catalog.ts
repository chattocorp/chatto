import { compareReleaseVersions } from '@chatto/client/server/compatibility';
import type { ReleaseHighlightState } from '@chatto/client/api/account';

/** Separate publication collections match the user and administration surfaces. */
export type HighlightSurface = 'users' | 'admins';
export type ReleaseHighlight = {
  /** Stable within its surface. Text corrections must retain this ID. */
  id: string;
  version: string;
  titleKey: string;
  /** Minimum server release needed for the described behavior, if any. */
  serverVersion?: string;
};

/** Curated reader-facing changes only; never generate this from the raw changelog. */
export const highlightCollections: Record<HighlightSurface, readonly ReleaseHighlight[]> = {
  users: [{ id: 'help-guides', version: '0.5.0', titleKey: 'ui.release_highlights.user_title' }],
  admins: [{ id: 'bot-guide', version: '0.5.0', titleKey: 'ui.release_highlights.admin_title' }]
};

/** Keep unknown releases out of positive feature claims; frontend-only guides need no server. */
export function applicableHighlights(
  surface: HighlightSurface,
  frontendVersion: string,
  serverVersion?: string,
  entries = highlightCollections[surface]
) {
  return entries
    .filter((entry) => {
      const frontend = compareReleaseVersions(frontendVersion, entry.version);
      if (frontend === null || frontend < 0) return false;
      if (!entry.serverVersion) return true;
      const server = compareReleaseVersions(serverVersion ?? '', entry.serverVersion);
      return server !== null && server >= 0;
    })
    .toSorted((a, b) => compareReleaseVersions(b.version, a.version) ?? 0);
}

/** First encounters announce the latest applicable release, not the complete archive. */
export function unreadHighlights(
  surface: HighlightSurface,
  entries: readonly ReleaseHighlight[],
  state?: ReleaseHighlightState
) {
  const baseline =
    (surface === 'users' ? state?.usersSinceVersion : state?.adminsSinceVersion) ??
    entries[0]?.version;
  const dismissed = new Set(state?.dismissedIds ?? []);
  return entries.filter(
    (entry) =>
      baseline !== undefined &&
      (compareReleaseVersions(entry.version, baseline) ?? -1) >= 0 &&
      !dismissed.has(`${surface}:${entry.id}`)
  );
}
