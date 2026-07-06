/**
 * Compute avatar initials from user display name or login.
 * - If displayName exists: extract up to 2 initials (first letter of first two words)
 * - If no displayName: use first character of login
 */
export function getAvatarInitials(
  displayName: string | null | undefined,
  login: string | null | undefined
): string {
  if (displayName?.trim()) {
    const words = displayName.trim().split(/\s+/).filter(Boolean);
    if (words.length >= 2) {
      return (words[0][0] + words[1][0]).toUpperCase();
    }
    return words[0][0].toUpperCase();
  }
  const firstLoginChar = login?.trim().charAt(0);
  return firstLoginChar ? firstLoginChar.toUpperCase() : '?';
}
