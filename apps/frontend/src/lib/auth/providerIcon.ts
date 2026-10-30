/** Shared provider icons for sign-in, account settings, and registration settings. */
export function providerIcon(type: string): string {
  switch (type) {
    case 'github':
      return 'icon-[mdi--github]';
    case 'gitlab':
      return 'icon-[mdi--gitlab]';
    case 'google':
      return 'icon-[mdi--google]';
    case 'discord':
      return 'icon-[mdi--discord]';
    default:
      return 'icon-[mdi--shield-account]';
  }
}
