import { beforeEach, describe, expect, it, vi } from 'vitest';

beforeEach(() => {
  vi.resetModules();
});

/** Fresh runtime and loader spies keep earlier tests from warming the catalogs. */
async function setup() {
  const catalogs = await import('./catalogs');
  const { managementCatalogSections } = await import('./catalogSections.js');
  const loaders = Object.fromEntries(
    managementCatalogSections.map((section) => [
      section,
      Object.fromEntries(
        Object.keys(catalogs.catalogLoaders[section]).map((locale) => [
          locale,
          vi.spyOn(catalogs.catalogLoaders[section], locale)
        ])
      )
    ])
  );
  const messages = await import('./messages');
  const runtime = await import('./runtime');
  return { ...catalogs, ...messages, ...runtime, loaders, managementCatalogSections };
}

describe('route translation boundaries', () => {
  it('does not invalidate translations when the requested sections are already active', async () => {
    const app = await setup();
    const { getI18nRevision } = await import('./state.svelte');
    await app.preloadChatLocaleMessages();
    const chatRevision = getI18nRevision();
    await Promise.all([app.preloadChatLocaleMessages(), app.preloadChatLocaleMessages()]);
    expect(getI18nRevision()).toBe(chatRevision);
    await app.preloadChatLocaleMessages(true);
    const managementRevision = getI18nRevision();
    await app.preloadChatLocaleMessages(true);
    expect(getI18nRevision()).toBe(managementRevision);
  });

  it('deactivates management without changing visible text or invalidating translations', async () => {
    const app = await setup();
    const { getI18nRevision } = await import('./state.svelte');
    await app.preloadChatLocaleMessages(true);
    const revision = getI18nRevision();
    await app.deactivateManagementLocaleMessages();
    await app.deactivateManagementLocaleMessages();
    expect(getI18nRevision()).toBe(revision);
    expect(app.m('admin.system.started')).toBe('Started');
    await app.setLocale('de-DE');
    expect(app.loaders.admin['de-DE']).not.toHaveBeenCalled();
    await app.preloadChatLocaleMessages(true);
    expect(app.m('admin.system.started')).toBe('Gestartet');
  });

  it.each(['en-GB', 'de-DE', 'de-AT'] as const)(
    'defers management, including English fallback, for %s',
    async (locale) => {
      const app = await setup();
      await app.preloadPublicLocaleMessages();
      await app.setLocale(locale);
      await app.preloadChatLocaleMessages();
      for (const section of app.managementCatalogSections) {
        expect(app.initialBaseCatalogs[section]).toBeUndefined();
        for (const loader of Object.values(app.loaders[section]))
          expect(loader).not.toHaveBeenCalled();
      }
      expect(app.m('chat.management_nav.general')).not.toMatch(/^⟦/);
      expect(app.m('room.management.create_room')).not.toMatch(/^⟦/);
      expect(app.m('permissions.descriptions.room_create')).not.toMatch(/^⟦/);
      expect(app.m('common.message_of_the_day')).not.toMatch(/^⟦/);
      expect(app.m('invite_links.create')).not.toMatch(/^⟦/);
      expect(app.m('invite_links.max_uses')).not.toMatch(/^⟦/);
      expect(app.m('invite_links.created')).not.toMatch(/^⟦/);
      expect(app.m('invite_links.copy')).not.toMatch(/^⟦/);

      await app.preloadChatLocaleMessages(true);
      expect(app.m('admin.system.started')).not.toMatch(/^⟦/);
      for (const section of app.managementCatalogSections) {
        expect(app.loaders[section]['en-GB']).toHaveBeenCalledOnce();
        // Regional overlays can omit a whole section and use their parent.
        const selected = app.loaders[section][locale];
        if (locale !== 'en-GB' && selected) expect(selected).toHaveBeenCalledOnce();
        if (locale === 'de-AT') expect(app.loaders[section]['de-DE']).toHaveBeenCalledOnce();
      }
      await app.preloadChatLocaleMessages(false);
      await app.preloadChatLocaleMessages(true);
      expect(app.loaders.admin['en-GB']).toHaveBeenCalledOnce();
    }
  );

  it('loads management on language changes only while its sections are active', async () => {
    const app = await setup();
    await app.preloadPublicLocaleMessages();
    await app.preloadChatLocaleMessages(true);
    await app.setLocale('de-DE');
    expect(app.m('admin.system.started')).toBe('Gestartet');
    expect(app.loaders.admin['de-DE']).toHaveBeenCalledOnce();
    await app.preloadChatLocaleMessages(false);
    await app.setLocale('fr-FR');
    expect(app.loaders.admin['fr-FR']).not.toHaveBeenCalled();
    await app.preloadChatLocaleMessages(true);
    expect(app.loaders.admin['fr-FR']).toHaveBeenCalledOnce();
  });

  it('retries failed management downloads without breaking shared messages', async () => {
    const app = await setup();
    await app.preloadPublicLocaleMessages();
    await app.preloadChatLocaleMessages();
    app.loaders.admin['en-GB'].mockRejectedValueOnce(new Error('offline'));
    await expect(app.preloadChatLocaleMessages(true)).rejects.toThrow('offline');
    expect(app.m('common.cancel')).toBe('Cancel');
    expect(app.m('admin.system.started')).toBe('⟦admin.system.started⟧');
    await app.preloadChatLocaleMessages(true);
    expect(app.m('admin.system.started')).toBe('Started');
    expect(app.loaders.admin['en-GB']).toHaveBeenCalledTimes(2);
  });

  it('serializes a locale switch with entry to management', async () => {
    const app = await setup();
    await app.preloadPublicLocaleMessages();
    await Promise.all([app.preloadChatLocaleMessages(true), app.setLocale('de-DE')]);
    expect(app.getLocale()).toBe('de-DE');
    expect(app.m('admin.system.started')).toBe('Gestartet');
  });
});
