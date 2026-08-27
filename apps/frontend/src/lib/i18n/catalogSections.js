export const catalogSections = /** @type {const} */ ([
  'add_server',
  'admin',
  'auth',
  'chat',
  'common',
  'composer',
  'emoji',
  'error_page',
  'media',
  'message_preview',
  'preview',
  'quick_switcher',
  'rbac',
  'room',
  'room_list',
  'search',
  'server_settings',
  'settings',
  'ui',
  'voice',
  'welcome'
]);

/** Catalog sections needed before the public application shell renders. */
export const publicCatalogSections = /** @type {const} */ ([
  'add_server',
  'auth',
  'chat',
  'common',
  'error_page',
  'room',
  'settings',
  'ui',
  'voice',
  'welcome'
]);

/** @param {(typeof catalogSections)[number]} section */
export function isPublicCatalogSection(section) {
  return /** @type {readonly string[]} */ (publicCatalogSections).includes(section);
}
