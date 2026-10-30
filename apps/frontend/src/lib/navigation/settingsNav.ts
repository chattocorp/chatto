import { resolve } from '$app/paths';
import { m } from '$lib/i18n/messages';
import { serverStorageKey } from '@chatto/client/storage/serverStorage';
import type { NavGroup, NavItem } from '$lib/components/SidebarNav.svelte';

/** Shared groups for the settings sidebar and Overview. Call from a reactive derivation. */
export function getSettingsNavGroups(
  serverId: string,
  serverSegment: string,
  managementNavItems: NavItem[]
): NavGroup[] {
  const userPreferenceNavItems = [
    {
      href: resolve('/chat/[serverId]/settings/account', { serverId: serverSegment }),
      label: m('settings.nav.account'),
      icon: 'iconify icon-[uil--setting]'
    },
    {
      href: resolve('/chat/[serverId]/settings/profile', { serverId: serverSegment }),
      label: m('settings.nav.profile'),
      icon: 'iconify icon-[uil--user]'
    },
    {
      href: resolve('/chat/[serverId]/settings/time', { serverId: serverSegment }),
      label: m('settings.preferences.title'),
      icon: 'iconify icon-[uil--clock]'
    },
    {
      href: resolve('/chat/[serverId]/settings/notifications', { serverId: serverSegment }),
      label: m('settings.nav.notifications'),
      icon: 'iconify icon-[uil--bell]'
    }
  ];
  const appPreferenceNavItems = [
    {
      href: resolve('/chat/[serverId]/settings/appearance', { serverId: serverSegment }),
      label: m('settings.app_preferences.appearance.title'),
      icon: 'iconify icon-[uil--palette]'
    },
    {
      href: resolve('/chat/[serverId]/settings/language', { serverId: serverSegment }),
      label: m('settings.preferences.language.title'),
      icon: 'iconify icon-[uil--language]'
    },
    {
      href: resolve('/chat/[serverId]/settings/composer', { serverId: serverSegment }),
      label: m('settings.app_preferences.composer.title'),
      icon: 'iconify icon-[uil--edit]'
    },
    {
      href: resolve('/chat/[serverId]/settings/voice', { serverId: serverSegment }),
      label: m('voice.preferences.title'),
      icon: 'iconify icon-[uil--headphones]'
    }
  ];

  return [
    {
      label: m('settings.nav.app_preferences'),
      items: appPreferenceNavItems,
      persistKey: serverStorageKey(serverId, 'collapsible:settings:app-preferences')
    },
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
