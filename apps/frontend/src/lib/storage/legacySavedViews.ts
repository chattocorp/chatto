// SPDX-License-Identifier: Apache-2.0

/** IndexedDB database that 0.5 beta clients used for saved chat views. */
const LEGACY_SAVED_VIEWS_DATABASE = 'chatto-saved-views';

/**
 * Delete the saved chat views that 0.5 beta clients stored on this device.
 * The client no longer stores private chat data in IndexedDB. A tab that runs
 * an older client version can create the database again, so every page load
 * repeats this deletion. Another open connection only delays the deletion
 * until that connection closes; this call does not wait for it.
 */
export function deleteLegacySavedViews(): void {
  if (typeof indexedDB === 'undefined') return;
  try {
    indexedDB.deleteDatabase(LEGACY_SAVED_VIEWS_DATABASE);
  } catch {
    // Storage can be unavailable, for example in some private browsing modes.
  }
}
