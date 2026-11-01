import { resolve } from '$app/paths';
import { m } from '$lib/i18n/messages';
import { serverStorageKey } from '@chatto/client/storage/serverStorage';
import type { NavGroup, NavItem } from '$lib/components/SidebarNav.svelte';
import { desktopUpdates } from '$lib/desktop/updates.svelte';

/** Navigation details shared by the sidebar and the descriptive Overview cards. */
type SettingsNavItem = NavItem & { description?: string };
/** One collapsible settings section and its Overview cards. */
export type SettingsNavGroup = Omit<NavGroup, 'items'> & { items: SettingsNavItem[] };

/**
 * Shared groups for the settings sidebar and Overview. Call from a reactive derivation.
 * Without a server, only preferences that do not need a session are included.
 */
export function getSettingsNavGroups(server?: {
  serverId: string;
  serverSegment: string;
  managementNavItems: SettingsNavItem[];
}): SettingsNavGroup[] {
  const appPreferences = {
    label: m('settings.nav.app_preferences'),
    items: getAppPreferenceNavItems(server?.serverSegment),
    persistKey: server
      ? serverStorageKey(server.serverId, 'collapsible:settings:app-preferences')
      : 'collapsible:settings:app-preferences'
  };
  if (!server) return [appPreferences];

  const { serverId, serverSegment, managementNavItems } = server;
  const userPreferenceNavItems = [
    {
      href: resolve('/chat/[serverId]/settings/account', { serverId: serverSegment }),
      label: m('settings.nav.account'),
      description: m('settings.overview.descriptions.account'),
      icon: 'iconify icon-[uil--setting]'
    },
    {
      href: resolve('/chat/[serverId]/settings/profile', { serverId: serverSegment }),
      label: m('settings.nav.profile'),
      description: m('settings.overview.descriptions.profile'),
      icon: 'iconify icon-[uil--user]'
    },
    {
      href: resolve('/chat/[serverId]/settings/time', { serverId: serverSegment }),
      label: m('settings.preferences.title'),
      description: m('settings.overview.descriptions.time'),
      icon: 'iconify icon-[uil--clock]'
    },
    {
      href: resolve('/chat/[serverId]/settings/notifications', { serverId: serverSegment }),
      label: m('settings.nav.notifications'),
      description: m('settings.overview.descriptions.notifications'),
      icon: 'iconify icon-[uil--bell]'
    }
  ];
  return [
    appPreferences,
    {
      label: m('settings.nav.your_account'),
      items: userPreferenceNavItems,
      persistKey: serverStorageKey(serverId, 'collapsible:settings:your-account')
    },
    {
      label: m('settings.nav.server_configuration'),
      items: managementNavItems,
      persistKey: serverStorageKey(serverId, 'collapsible:settings:server-configuration')
    }
  ];
}

/** App preference destinations. Omit the server segment for signed-out navigation. */
export function getAppPreferenceNavItems(serverSegment?: string): SettingsNavItem[] {
  const appPreferenceNavItems = [
    {
      href: serverSegment
        ? resolve('/chat/[serverId]/settings/appearance', { serverId: serverSegment })
        : resolve('/chat/preferences/appearance'),
      label: m('settings.app_preferences.appearance.title'),
      description: m('settings.overview.descriptions.appearance'),
      icon: 'iconify icon-[uil--palette]'
    },
    {
      href: serverSegment
        ? resolve('/chat/[serverId]/settings/language', { serverId: serverSegment })
        : resolve('/chat/preferences/language'),
      label: m('settings.preferences.language.title'),
      description: m('settings.overview.descriptions.language'),
      icon: 'iconify icon-[uil--language]'
    },
    {
      href: serverSegment
        ? resolve('/chat/[serverId]/settings/composer', { serverId: serverSegment })
        : resolve('/chat/preferences/composer'),
      label: m('settings.app_preferences.composer.title'),
      description: m('settings.overview.descriptions.composer'),
      icon: 'iconify icon-[uil--edit]'
    }
  ];

  // Voice preferences use the server's call state and require a session.
  if (serverSegment) {
    appPreferenceNavItems.push({
      href: resolve('/chat/[serverId]/settings/voice', { serverId: serverSegment }),
      label: m('voice.preferences.title'),
      description: m('settings.overview.descriptions.voice'),
      icon: 'iconify icon-[uil--headphones]'
    });
  }

  if (desktopUpdates.available)
    appPreferenceNavItems.push({
      href: resolve('/chat/preferences/desktop'),
      label: m('settings.desktop.title'),
      description: m('settings.desktop.subtitle'),
      icon: 'iconify icon-[uil--desktop]'
    });

  return appPreferenceNavItems;
}
