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
  'permissions',
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

/** Catalogs used only by management routes, including their English fallback. */
export const managementCatalogSections = /** @type {const} */ ([
  'admin',
  'rbac',
  'server_settings'
]);

/** @param {(typeof catalogSections)[number]} section */
export function isManagementCatalogSection(section) {
  return /** @type {readonly string[]} */ (managementCatalogSections).includes(section);
}

/** Public and chat catalogs needed outside management. */
export const chatCatalogSections = catalogSections.filter(
  (section) => !isManagementCatalogSection(section)
);

/** @param {(typeof catalogSections)[number]} section */
export function catalogBoundary(section) {
  return isManagementCatalogSection(section)
    ? 'management'
    : isPublicCatalogSection(section)
      ? 'public'
      : 'chat';
}
