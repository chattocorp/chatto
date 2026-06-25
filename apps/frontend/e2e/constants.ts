/**
 * Standardized timeout constants for E2E tests.
 *
 * Use these instead of magic numbers to ensure consistency across tests
 * and make it easier to tune timeouts based on CI performance.
 *
 * Guidelines:
 * - Prefer the smallest timeout that reliably passes
 * - Use REALTIME_EVENT for multi-user scenarios where WebSocket delivery is involved
 * - Use COMPLEX_OPERATION only for operations that genuinely take a long time
 */
export const TIMEOUTS = {
  /**
   * Fast UI operations - element visibility, focus, simple state changes.
   * Use when the operation should complete almost immediately.
   */
  UI_FAST: 2000,

  /**
   * Standard UI operations - navigation, form submissions, single-user async operations.
   * This is the default for most assertions.
   */
  UI_STANDARD: 5000,

  /**
   * Real-time events - multi-user scenarios involving WebSocket message delivery.
   * Use when one user's action needs to be seen by another user.
   */
  REALTIME_EVENT: 10000,

  /**
   * Complex operations - file uploads, long polling, operations with multiple retries.
   * Use sparingly and only when genuinely needed.
   */
  COMPLEX_OPERATION: 15000,

  /**
   * Polling with retries - for .toPass() assertions that need multiple attempts.
   * Use with intervals: [500, 1000, 2000] for exponential backoff.
   */
  POLLING_EXTENDED: 20000,

  /**
   * Server mutation completion - wait for async mutations (like markRoomAsRead) to
   * complete and sync to server-side state. Use when a subsequent operation depends
   * on server state that was updated by a mutation triggered by page load.
   */
  SERVER_MUTATION_SYNC: 2000,

  /**
   * Scroll settling - wait for virtua to process scroll position changes
   * and complete any measurement-based corrections. Use between scroll
   * wheel events or after triggering scroll-based pagination.
   */
  SCROLL_SETTLE: 150,

  /**
   * Layout settling - wait for CSS layout changes (e.g., viewport resize,
   * sidebar toggle) to complete. Use after setViewportSize or similar.
   */
  LAYOUT_SETTLE: 200,

  /**
   * Network simulation - minimum time to hold network offline so in-flight
   * requests fail and the client detects disconnection.
   */
  NETWORK_OFFLINE: 1500
} as const;

/**
 * Standard intervals for .toPass() polling assertions.
 * Uses exponential backoff pattern.
 */
export const POLLING_INTERVALS = [500, 1000, 2000] as const;

/**
 * Fast polling intervals for scroll and UI settling assertions.
 */
export const FAST_INTERVALS = [50, 100, 200] as const;
