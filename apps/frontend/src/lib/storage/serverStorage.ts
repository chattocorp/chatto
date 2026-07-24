/**
 * Utility for namespacing localStorage keys by instance ID.
 * Prevents data collisions when multiple Chatto instances share a browser.
 */

/**
 * Build a localStorage key scoped to a specific instance.
 *
 * @example serverStorageKey("chat-example-com", "lastRooms") → "chatto:i:chat-example-com:lastRooms"
 */
export function serverStorageKey(serverId: string, key: string): string {
	return `chatto:i:${serverId}:${key}`;
}
