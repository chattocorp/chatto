/**
 * Shared constants for the Chatto frontend.
 *
 * IMPORTANT: The DM_SPACE_ID must match the backend constant in
 * cli/internal/core/dm.go (DMSpaceID = "DM")
 *
 * Note: GraphQL queries use literal strings (e.g., space(id: "DM"))
 * and cannot reference this constant. When searching for usages, also
 * grep for the literal string "DM" in .svelte and .ts files.
 */

/**
 * The well-known space ID for direct messages.
 * DM conversations are rooms within this system space.
 */
export const DM_SPACE_ID = 'DM';

/**
 * Kind-discriminator string for non-DM rooms. Matches the backend
 * `core.ServerSpaceID = "server"` constant — see ADR-030.
 *
 * Only test fixtures and a few legacy helpers need this; production code
 * paths don't construct spaceIDs anymore.
 */
export const SERVER_SPACE_ID = 'server';
