/**
 * Whether the current platform conventionally uses the Command key for
 * keyboard shortcuts. Accepting a navigator-shaped value keeps the platform
 * detection deterministic in tests and safe during server-side rendering.
 */
export function usesAppleShortcutModifier(
  navigatorLike:
    | { platform?: string; userAgentData?: { platform?: string } }
    | undefined = typeof navigator === 'undefined' ? undefined : navigator
): boolean {
  const platform = navigatorLike?.userAgentData?.platform ?? navigatorLike?.platform ?? '';
  return /Mac|iPhone|iPad|iPod/i.test(platform);
}
