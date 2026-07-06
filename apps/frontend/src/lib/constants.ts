/**
 * Shared constants for the Chatto frontend.
 */

/**
 * Kind-discriminator string for non-DM rooms. Matches the backend
 * `core.LegacyServerSpaceID = "server"` constant — see ADR-030.
 *
 * Only test fixtures and a few legacy helpers need this; production code
 * paths don't construct spaceIDs anymore.
 */
export const SERVER_SPACE_ID = 'server';
