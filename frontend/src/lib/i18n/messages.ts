import * as enMessages from '$lib/paraglide/messages/en.js';
import { getLocale, type Locale, type LocalizedString } from '$lib/paraglide/runtime';
import { getReactiveLocale, setReactiveLocale } from './state.svelte';

type LocaleMessages = typeof enMessages;
type EmptyInputs = Record<string, never>;

// Keep app code on this facade. The generated all-message index imports every
// locale eagerly; direct locale-module imports let Vite split non-base locales.
let activeLocale: Locale = 'en';
let activeMessages: LocaleMessages = enMessages;

const loadedLocales = new Map<Locale, Promise<LocaleMessages>>([
  ['en', Promise.resolve(enMessages)]
]);

function loadLocaleModule(locale: Locale): Promise<LocaleMessages> {
  const existing = loadedLocales.get(locale);
  if (existing) return existing;

  const loading =
    locale === 'de'
      ? (import('$lib/paraglide/messages/de.js') as Promise<LocaleMessages>)
      : Promise.resolve(enMessages);

  loadedLocales.set(locale, loading);
  return loading;
}

export async function loadLocaleMessages(locale: Locale): Promise<void> {
  activeMessages = await loadLocaleModule(locale);
  activeLocale = locale;
}

export async function preloadActiveLocaleMessages(): Promise<void> {
  const locale = getLocale();
  await loadLocaleMessages(locale);
  setReactiveLocale(locale);
}

function messages(): LocaleMessages {
  const locale = getReactiveLocale();

  if (locale === activeLocale) return activeMessages;
  if (locale === 'en') {
    activeLocale = 'en';
    activeMessages = enMessages;
    return activeMessages;
  }

  return activeMessages;
}

function empty(): EmptyInputs {
  return {};
}

const commonCancel = (): LocalizedString => messages().common_cancel(empty());
const commonCloseSidebar = (): LocalizedString => messages().common_close_sidebar(empty());
const settingsNavTitle = (): LocalizedString => messages().settings_nav_title(empty());
const settingsNavBackToServer = (): LocalizedString =>
  messages().settings_nav_back_to_server(empty());
const settingsNavProfile = (): LocalizedString => messages().settings_nav_profile(empty());
const settingsNavDisplay = (): LocalizedString => messages().settings_nav_display(empty());
const settingsNavNotifications = (): LocalizedString =>
  messages().settings_nav_notifications(empty());
const settingsNavAccount = (): LocalizedString => messages().settings_nav_account(empty());
const settingsProfileTitle = (): LocalizedString => messages().settings_profile_title(empty());
const settingsProfileSubtitle = (): LocalizedString =>
  messages().settings_profile_subtitle(empty());
const settingsProfileAvatarTitle = (): LocalizedString =>
  messages().settings_profile_avatar_title(empty());
const settingsProfileAvatarDropTitle = (): LocalizedString =>
  messages().settings_profile_avatar_drop_title(empty());
const settingsProfileAvatarDropSubtitle = (): LocalizedString =>
  messages().settings_profile_avatar_drop_subtitle(empty());
const settingsProfileAvatarAlt = (): LocalizedString =>
  messages().settings_profile_avatar_alt(empty());
const settingsProfileAvatarDescription = (): LocalizedString =>
  messages().settings_profile_avatar_description(empty());
const settingsProfileAvatarUploading = (): LocalizedString =>
  messages().settings_profile_avatar_uploading(empty());
const settingsProfileAvatarUpload = (): LocalizedString =>
  messages().settings_profile_avatar_upload(empty());
const settingsProfileAvatarChange = (): LocalizedString =>
  messages().settings_profile_avatar_change(empty());
const settingsProfileAvatarRemoving = (): LocalizedString =>
  messages().settings_profile_avatar_removing(empty());
const settingsProfileAvatarRemove = (): LocalizedString =>
  messages().settings_profile_avatar_remove(empty());
const settingsProfileAvatarInvalidType = (): LocalizedString =>
  messages().settings_profile_avatar_invalid_type(empty());
const settingsProfileAvatarTooLarge = (): LocalizedString =>
  messages().settings_profile_avatar_too_large(empty());
const settingsProfileAvatarUploaded = (): LocalizedString =>
  messages().settings_profile_avatar_uploaded(empty());
const settingsProfileAvatarUploadFailed = (): LocalizedString =>
  messages().settings_profile_avatar_upload_failed(empty());
const settingsProfileAvatarRemoved = (): LocalizedString =>
  messages().settings_profile_avatar_removed(empty());
const settingsProfileAvatarDeleteFailed = (): LocalizedString =>
  messages().settings_profile_avatar_delete_failed(empty());
const settingsProfileDisplayNameLabel = (): LocalizedString =>
  messages().settings_profile_display_name_label(empty());
const settingsProfileDisplayNamePlaceholder = (): LocalizedString =>
  messages().settings_profile_display_name_placeholder(empty());
const settingsProfileDisplayNameInvalid = (): LocalizedString =>
  messages().settings_profile_display_name_invalid(empty());
const settingsProfileUsernameLabel = (): LocalizedString =>
  messages().settings_profile_username_label(empty());
const settingsProfileUsernamePlaceholder = (): LocalizedString =>
  messages().settings_profile_username_placeholder(empty());
const settingsProfileUsernameInvalid = (): LocalizedString =>
  messages().settings_profile_username_invalid(empty());
const settingsProfileUsernameCooldownError = (
  inputs: Parameters<LocaleMessages['settings_profile_username_cooldown_error']>[0]
): LocalizedString => messages().settings_profile_username_cooldown_error(inputs);
const settingsProfileUsernameCooldownNotice = (
  inputs: Parameters<LocaleMessages['settings_profile_username_cooldown_notice']>[0]
): LocalizedString => messages().settings_profile_username_cooldown_notice(inputs);
const settingsProfileUsernameConfirmTitle = (): LocalizedString =>
  messages().settings_profile_username_confirm_title(empty());
const settingsProfileUsernameConfirmPrompt = (
  inputs: Parameters<LocaleMessages['settings_profile_username_confirm_prompt']>[0]
): LocalizedString => messages().settings_profile_username_confirm_prompt(inputs);
const settingsProfileUsernameConfirmCooldown = (): LocalizedString =>
  messages().settings_profile_username_confirm_cooldown(empty());
const settingsProfileUsernameConfirmButton = (): LocalizedString =>
  messages().settings_profile_username_confirm_button(empty());
const settingsProfileSaved = (): LocalizedString => messages().settings_profile_saved(empty());
const settingsProfileSaveFailed = (): LocalizedString =>
  messages().settings_profile_save_failed(empty());
const settingsProfileSaveButton = (): LocalizedString =>
  messages().settings_profile_save_button(empty());
const settingsPreferencesTitle = (): LocalizedString =>
  messages().settings_preferences_title(empty());
const settingsPreferencesSubtitle = (): LocalizedString =>
  messages().settings_preferences_subtitle(empty());
const settingsPreferencesThemeTitle = (): LocalizedString =>
  messages().settings_preferences_theme_title(empty());
const settingsPreferencesThemeSystemLabel = (): LocalizedString =>
  messages().settings_preferences_theme_system_label(empty());
const settingsPreferencesThemeSystemDescription = (): LocalizedString =>
  messages().settings_preferences_theme_system_description(empty());
const settingsPreferencesThemeLightLabel = (): LocalizedString =>
  messages().settings_preferences_theme_light_label(empty());
const settingsPreferencesThemeLightDescription = (): LocalizedString =>
  messages().settings_preferences_theme_light_description(empty());
const settingsPreferencesThemeDarkLabel = (): LocalizedString =>
  messages().settings_preferences_theme_dark_label(empty());
const settingsPreferencesThemeDarkDescription = (): LocalizedString =>
  messages().settings_preferences_theme_dark_description(empty());
const settingsPreferencesLanguageTitle = (): LocalizedString =>
  messages().settings_preferences_language_title(empty());
const settingsPreferencesLanguageDescription = (): LocalizedString =>
  messages().settings_preferences_language_description(empty());
const settingsPreferencesLanguageEnglish = (): LocalizedString =>
  messages().settings_preferences_language_english(empty());
const settingsPreferencesLanguageGerman = (): LocalizedString =>
  messages().settings_preferences_language_german(empty());
const settingsPreferencesTimezoneTitle = (): LocalizedString =>
  messages().settings_preferences_timezone_title(empty());
const settingsPreferencesTimezoneDescription = (): LocalizedString =>
  messages().settings_preferences_timezone_description(empty());
const settingsPreferencesTimezoneBrowserDefault = (): LocalizedString =>
  messages().settings_preferences_timezone_browser_default(empty());
const settingsPreferencesTimezoneClear = (): LocalizedString =>
  messages().settings_preferences_timezone_clear(empty());
const settingsPreferencesTimezoneInvalid = (): LocalizedString =>
  messages().settings_preferences_timezone_invalid(empty());
const settingsPreferencesTimezoneMoreResults = (
  inputs: Parameters<LocaleMessages['settings_preferences_timezone_more_results']>[0]
): LocalizedString => messages().settings_preferences_timezone_more_results(inputs);
const settingsPreferencesTimezoneCurrentTime = (
  inputs: Parameters<LocaleMessages['settings_preferences_timezone_current_time']>[0]
): LocalizedString => messages().settings_preferences_timezone_current_time(inputs);
const settingsPreferencesTimeFormatTitle = (): LocalizedString =>
  messages().settings_preferences_time_format_title(empty());
const settingsPreferencesTimeFormatBrowserDefaultLabel = (): LocalizedString =>
  messages().settings_preferences_time_format_browser_default_label(empty());
const settingsPreferencesTimeFormatBrowserDefaultDescription = (): LocalizedString =>
  messages().settings_preferences_time_format_browser_default_description(empty());
const settingsPreferencesTimeFormat12hLabel = (): LocalizedString =>
  messages().settings_preferences_time_format_12h_label(empty());
const settingsPreferencesTimeFormat12hDescription = (): LocalizedString =>
  messages().settings_preferences_time_format_12h_description(empty());
const settingsPreferencesTimeFormat24hLabel = (): LocalizedString =>
  messages().settings_preferences_time_format_24h_label(empty());
const settingsPreferencesTimeFormat24hDescription = (): LocalizedString =>
  messages().settings_preferences_time_format_24h_description(empty());
const settingsPreferencesSaveButton = (): LocalizedString =>
  messages().settings_preferences_save_button(empty());
const settingsPreferencesSaved = (): LocalizedString =>
  messages().settings_preferences_saved(empty());
const settingsPreferencesSaveFailed = (): LocalizedString =>
  messages().settings_preferences_save_failed(empty());
const settingsNotificationsTitle = (): LocalizedString =>
  messages().settings_notifications_title(empty());
const settingsNotificationsSubtitle = (): LocalizedString =>
  messages().settings_notifications_subtitle(empty());
const settingsNotificationsPushTitle = (): LocalizedString =>
  messages().settings_notifications_push_title(empty());
const settingsNotificationsPushNotConfigured = (): LocalizedString =>
  messages().settings_notifications_push_not_configured(empty());
const settingsNotificationsPushNotSupported = (): LocalizedString =>
  messages().settings_notifications_push_not_supported(empty());
const settingsNotificationsPushBlockedTitle = (): LocalizedString =>
  messages().settings_notifications_push_blocked_title(empty());
const settingsNotificationsPushBlockedDescription = (): LocalizedString =>
  messages().settings_notifications_push_blocked_description(empty());
const settingsNotificationsPushEnabledTitle = (): LocalizedString =>
  messages().settings_notifications_push_enabled_title(empty());
const settingsNotificationsPushEnabledDescription = (): LocalizedString =>
  messages().settings_notifications_push_enabled_description(empty());
const settingsNotificationsPushEnableTitle = (): LocalizedString =>
  messages().settings_notifications_push_enable_title(empty());
const settingsNotificationsPushEnableDescription = (): LocalizedString =>
  messages().settings_notifications_push_enable_description(empty());
const settingsNotificationsPushEnableButton = (): LocalizedString =>
  messages().settings_notifications_push_enable_button(empty());
const settingsNotificationsPushEnabling = (): LocalizedString =>
  messages().settings_notifications_push_enabling(empty());
const settingsNotificationsPushBlockedError = (): LocalizedString =>
  messages().settings_notifications_push_blocked_error(empty());
const settingsNotificationsPushEnableFailed = (): LocalizedString =>
  messages().settings_notifications_push_enable_failed(empty());
const settingsNotificationsPushEnableError = (): LocalizedString =>
  messages().settings_notifications_push_enable_error(empty());
const settingsNotificationsLevelsLoading = (): LocalizedString =>
  messages().settings_notifications_levels_loading(empty());
const settingsNotificationsLevelsLoadFailed = (): LocalizedString =>
  messages().settings_notifications_levels_load_failed(empty());
const settingsNotificationsLevelsServerUpdated = (): LocalizedString =>
  messages().settings_notifications_levels_server_updated(empty());
const settingsNotificationsLevelsRoomUpdated = (): LocalizedString =>
  messages().settings_notifications_levels_room_updated(empty());
const settingsNotificationsLevelsUpdateFailed = (): LocalizedString =>
  messages().settings_notifications_levels_update_failed(empty());
const settingsNotificationsLevelsServerTitle = (): LocalizedString =>
  messages().settings_notifications_levels_server_title(empty());
const settingsNotificationsLevelsServerDescription = (): LocalizedString =>
  messages().settings_notifications_levels_server_description(empty());
const settingsNotificationsLevelsRoomTitle = (): LocalizedString =>
  messages().settings_notifications_levels_room_title(empty());
const settingsNotificationsLevelsRoomDescription = (
  inputs: Parameters<LocaleMessages['settings_notifications_levels_room_description']>[0]
): LocalizedString => messages().settings_notifications_levels_room_description(inputs);
const settingsNotificationsLevelsEffective = (
  inputs: Parameters<LocaleMessages['settings_notifications_levels_effective']>[0]
): LocalizedString => messages().settings_notifications_levels_effective(inputs);
const settingsNotificationsLevelsDefaultLabel = (): LocalizedString =>
  messages().settings_notifications_levels_default_label(empty());
const settingsNotificationsLevelsDefaultDescription = (): LocalizedString =>
  messages().settings_notifications_levels_default_description(empty());
const settingsNotificationsLevelsMutedLabel = (): LocalizedString =>
  messages().settings_notifications_levels_muted_label(empty());
const settingsNotificationsLevelsMutedDescription = (): LocalizedString =>
  messages().settings_notifications_levels_muted_description(empty());
const settingsNotificationsLevelsNormalLabel = (): LocalizedString =>
  messages().settings_notifications_levels_normal_label(empty());
const settingsNotificationsLevelsNormalDescription = (): LocalizedString =>
  messages().settings_notifications_levels_normal_description(empty());
const settingsNotificationsLevelsAllMessagesLabel = (): LocalizedString =>
  messages().settings_notifications_levels_all_messages_label(empty());
const settingsNotificationsLevelsAllMessagesDescription = (): LocalizedString =>
  messages().settings_notifications_levels_all_messages_description(empty());
const settingsNotificationsSoundTitle = (): LocalizedString =>
  messages().settings_notifications_sound_title(empty());
const settingsNotificationsSoundShapeTitle = (): LocalizedString =>
  messages().settings_notifications_sound_shape_title(empty());
const settingsNotificationsSoundPreview = (): LocalizedString =>
  messages().settings_notifications_sound_preview(empty());
const settingsNotificationsSoundReset = (): LocalizedString =>
  messages().settings_notifications_sound_reset(empty());
const settingsNotificationsSoundOff = (): LocalizedString =>
  messages().settings_notifications_sound_off(empty());
const settingsNotificationsSoundVolume = (): LocalizedString =>
  messages().settings_notifications_sound_volume(empty());
const settingsNotificationsSoundTinny = (): LocalizedString =>
  messages().settings_notifications_sound_tinny(empty());
const settingsNotificationsSoundMuffled = (): LocalizedString =>
  messages().settings_notifications_sound_muffled(empty());
const settingsNotificationsSoundEcho = (): LocalizedString =>
  messages().settings_notifications_sound_echo(empty());
const settingsNotificationsSoundReverb = (): LocalizedString =>
  messages().settings_notifications_sound_reverb(empty());
const settingsNotificationsSoundCrunch = (): LocalizedString =>
  messages().settings_notifications_sound_crunch(empty());
const settingsNotificationsSoundCategorySilent = (): LocalizedString =>
  messages().settings_notifications_sound_category_silent(empty());
const settingsNotificationsSoundCategorySimple = (): LocalizedString =>
  messages().settings_notifications_sound_category_simple(empty());
const settingsNotificationsSoundCategoryPlayful = (): LocalizedString =>
  messages().settings_notifications_sound_category_playful(empty());
const settingsNotificationsSoundCategoryRobots = (): LocalizedString =>
  messages().settings_notifications_sound_category_robots(empty());
const settingsNotificationsSoundCategoryMusical = (): LocalizedString =>
  messages().settings_notifications_sound_category_musical(empty());
const settingsNotificationsSoundCategoryHereBeDragons = (): LocalizedString =>
  messages().settings_notifications_sound_category_here_be_dragons(empty());
const settingsNotificationsSoundNameSilent = (): LocalizedString =>
  messages().settings_notifications_sound_name_silent(empty());
const settingsNotificationsSoundNameDing = (): LocalizedString =>
  messages().settings_notifications_sound_name_ding(empty());
const settingsNotificationsSoundNameChimeUp = (): LocalizedString =>
  messages().settings_notifications_sound_name_chime_up(empty());
const settingsNotificationsSoundNameChimeDown = (): LocalizedString =>
  messages().settings_notifications_sound_name_chime_down(empty());
const settingsNotificationsSoundNamePop = (): LocalizedString =>
  messages().settings_notifications_sound_name_pop(empty());
const settingsNotificationsSoundNameBubble = (): LocalizedString =>
  messages().settings_notifications_sound_name_bubble(empty());
const settingsNotificationsSoundNameRetro = (): LocalizedString =>
  messages().settings_notifications_sound_name_retro(empty());
const settingsNotificationsSoundNameCoin = (): LocalizedString =>
  messages().settings_notifications_sound_name_coin(empty());
const settingsNotificationsSoundNamePowerup = (): LocalizedString =>
  messages().settings_notifications_sound_name_powerup(empty());
const settingsNotificationsSoundNameFanfare = (): LocalizedString =>
  messages().settings_notifications_sound_name_fanfare(empty());
const settingsNotificationsSoundNameLaser = (): LocalizedString =>
  messages().settings_notifications_sound_name_laser(empty());
const settingsNotificationsSoundNameRobot = (): LocalizedString =>
  messages().settings_notifications_sound_name_robot(empty());
const settingsNotificationsSoundNameUfo = (): LocalizedString =>
  messages().settings_notifications_sound_name_ufo(empty());
const settingsNotificationsSoundNameBeepboop = (): LocalizedString =>
  messages().settings_notifications_sound_name_beepboop(empty());
const settingsNotificationsSoundNameDialup = (): LocalizedString =>
  messages().settings_notifications_sound_name_dialup(empty());
const settingsNotificationsSoundNameR2d2 = (): LocalizedString =>
  messages().settings_notifications_sound_name_r2d2(empty());
const settingsNotificationsSoundNameHarp = (): LocalizedString =>
  messages().settings_notifications_sound_name_harp(empty());
const settingsNotificationsSoundNameMusicBox = (): LocalizedString =>
  messages().settings_notifications_sound_name_music_box(empty());
const settingsNotificationsSoundNameCelesta = (): LocalizedString =>
  messages().settings_notifications_sound_name_celesta(empty());
const settingsNotificationsSoundNameSynth = (): LocalizedString =>
  messages().settings_notifications_sound_name_synth(empty());
const settingsNotificationsSoundNameOrchestra = (): LocalizedString =>
  messages().settings_notifications_sound_name_orchestra(empty());
const settingsNotificationsSoundNameLaCucaracha = (): LocalizedString =>
  messages().settings_notifications_sound_name_la_cucaracha(empty());
const settingsNotificationsSoundNameChaos = (): LocalizedString =>
  messages().settings_notifications_sound_name_chaos(empty());
const settingsNotificationsSoundNameGlitch = (): LocalizedString =>
  messages().settings_notifications_sound_name_glitch(empty());
const settingsNotificationsSoundNameSiren = (): LocalizedString =>
  messages().settings_notifications_sound_name_siren(empty());
const settingsNotificationsSoundNameDubstep = (): LocalizedString =>
  messages().settings_notifications_sound_name_dubstep(empty());
const settingsNotificationsSoundNameCircus = (): LocalizedString =>
  messages().settings_notifications_sound_name_circus(empty());
const settingsAccountTitle = (): LocalizedString => messages().settings_account_title(empty());
const settingsAccountSubtitle = (): LocalizedString =>
  messages().settings_account_subtitle(empty());
const settingsAccountInfoTitle = (): LocalizedString =>
  messages().settings_account_info_title(empty());
const settingsAccountUsername = (): LocalizedString =>
  messages().settings_account_username(empty());
const settingsAccountDisplayName = (): LocalizedString =>
  messages().settings_account_display_name(empty());
const settingsAccountDangerTitle = (): LocalizedString =>
  messages().settings_account_danger_title(empty());
const settingsAccountDangerDescription = (): LocalizedString =>
  messages().settings_account_danger_description(empty());
const settingsAccountDeleteButton = (): LocalizedString =>
  messages().settings_account_delete_button(empty());
const settingsAccountDeleteFailed = (): LocalizedString =>
  messages().settings_account_delete_failed(empty());
const settingsAccountDeleteRequestFailed = (): LocalizedString =>
  messages().settings_account_delete_request_failed(empty());
const settingsAccountDeleteModalTitle = (): LocalizedString =>
  messages().settings_account_delete_modal_title(empty());
const settingsAccountDeleteModalWarningLabel = (): LocalizedString =>
  messages().settings_account_delete_modal_warning_label(empty());
const settingsAccountDeleteModalWarningText = (): LocalizedString =>
  messages().settings_account_delete_modal_warning_text(empty());
const settingsAccountDeleteModalIntro = (): LocalizedString =>
  messages().settings_account_delete_modal_intro(empty());
const settingsAccountDeleteModalRemoveFromRooms = (): LocalizedString =>
  messages().settings_account_delete_modal_remove_from_rooms(empty());
const settingsAccountDeleteModalDeleteMessages = (): LocalizedString =>
  messages().settings_account_delete_modal_delete_messages(empty());
const settingsAccountDeleteModalDeleteProfile = (): LocalizedString =>
  messages().settings_account_delete_modal_delete_profile(empty());
const settingsAccountDeleteModalConfirmLabel = (): LocalizedString =>
  messages().settings_account_delete_modal_confirm_label(empty());
const settingsAccountDeleteModalConfirmPlaceholder = (): LocalizedString =>
  messages().settings_account_delete_modal_confirm_placeholder(empty());
const settingsAccountDeleteModalCancel = (): LocalizedString =>
  messages().settings_account_delete_modal_cancel(empty());
const settingsAccountDeleteModalDeleting = (): LocalizedString =>
  messages().settings_account_delete_modal_deleting(empty());

export { commonCancel as 'common.cancel' };
export { commonCloseSidebar as 'common.close_sidebar' };
export { settingsNavTitle as 'settings.nav.title' };
export { settingsNavBackToServer as 'settings.nav.back_to_server' };
export { settingsNavProfile as 'settings.nav.profile' };
export { settingsNavDisplay as 'settings.nav.display' };
export { settingsNavNotifications as 'settings.nav.notifications' };
export { settingsNavAccount as 'settings.nav.account' };
export { settingsProfileTitle as 'settings.profile.title' };
export { settingsProfileSubtitle as 'settings.profile.subtitle' };
export { settingsProfileAvatarTitle as 'settings.profile.avatar.title' };
export { settingsProfileAvatarDropTitle as 'settings.profile.avatar.drop_title' };
export { settingsProfileAvatarDropSubtitle as 'settings.profile.avatar.drop_subtitle' };
export { settingsProfileAvatarAlt as 'settings.profile.avatar.alt' };
export { settingsProfileAvatarDescription as 'settings.profile.avatar.description' };
export { settingsProfileAvatarUploading as 'settings.profile.avatar.uploading' };
export { settingsProfileAvatarUpload as 'settings.profile.avatar.upload' };
export { settingsProfileAvatarChange as 'settings.profile.avatar.change' };
export { settingsProfileAvatarRemoving as 'settings.profile.avatar.removing' };
export { settingsProfileAvatarRemove as 'settings.profile.avatar.remove' };
export { settingsProfileAvatarInvalidType as 'settings.profile.avatar.invalid_type' };
export { settingsProfileAvatarTooLarge as 'settings.profile.avatar.too_large' };
export { settingsProfileAvatarUploaded as 'settings.profile.avatar.uploaded' };
export { settingsProfileAvatarUploadFailed as 'settings.profile.avatar.upload_failed' };
export { settingsProfileAvatarRemoved as 'settings.profile.avatar.removed' };
export { settingsProfileAvatarDeleteFailed as 'settings.profile.avatar.delete_failed' };
export { settingsProfileDisplayNameLabel as 'settings.profile.display_name.label' };
export { settingsProfileDisplayNamePlaceholder as 'settings.profile.display_name.placeholder' };
export { settingsProfileDisplayNameInvalid as 'settings.profile.display_name.invalid' };
export { settingsProfileUsernameLabel as 'settings.profile.username.label' };
export { settingsProfileUsernamePlaceholder as 'settings.profile.username.placeholder' };
export { settingsProfileUsernameInvalid as 'settings.profile.username.invalid' };
export { settingsProfileUsernameCooldownError as 'settings.profile.username.cooldown_error' };
export { settingsProfileUsernameCooldownNotice as 'settings.profile.username.cooldown_notice' };
export { settingsProfileUsernameConfirmTitle as 'settings.profile.username.confirm_title' };
export { settingsProfileUsernameConfirmPrompt as 'settings.profile.username.confirm_prompt' };
export { settingsProfileUsernameConfirmCooldown as 'settings.profile.username.confirm_cooldown' };
export { settingsProfileUsernameConfirmButton as 'settings.profile.username.confirm_button' };
export { settingsProfileSaved as 'settings.profile.saved' };
export { settingsProfileSaveFailed as 'settings.profile.save_failed' };
export { settingsProfileSaveButton as 'settings.profile.save_button' };
export { settingsPreferencesTitle as 'settings.preferences.title' };
export { settingsPreferencesSubtitle as 'settings.preferences.subtitle' };
export { settingsPreferencesThemeTitle as 'settings.preferences.theme.title' };
export { settingsPreferencesThemeSystemLabel as 'settings.preferences.theme.system.label' };
export { settingsPreferencesThemeSystemDescription as 'settings.preferences.theme.system.description' };
export { settingsPreferencesThemeLightLabel as 'settings.preferences.theme.light.label' };
export { settingsPreferencesThemeLightDescription as 'settings.preferences.theme.light.description' };
export { settingsPreferencesThemeDarkLabel as 'settings.preferences.theme.dark.label' };
export { settingsPreferencesThemeDarkDescription as 'settings.preferences.theme.dark.description' };
export { settingsPreferencesLanguageTitle as 'settings.preferences.language.title' };
export { settingsPreferencesLanguageDescription as 'settings.preferences.language.description' };
export { settingsPreferencesLanguageEnglish as 'settings.preferences.language.english' };
export { settingsPreferencesLanguageGerman as 'settings.preferences.language.german' };
export { settingsPreferencesTimezoneTitle as 'settings.preferences.timezone.title' };
export { settingsPreferencesTimezoneDescription as 'settings.preferences.timezone.description' };
export { settingsPreferencesTimezoneBrowserDefault as 'settings.preferences.timezone.browser_default' };
export { settingsPreferencesTimezoneClear as 'settings.preferences.timezone.clear' };
export { settingsPreferencesTimezoneInvalid as 'settings.preferences.timezone.invalid' };
export { settingsPreferencesTimezoneMoreResults as 'settings.preferences.timezone.more_results' };
export { settingsPreferencesTimezoneCurrentTime as 'settings.preferences.timezone.current_time' };
export { settingsPreferencesTimeFormatTitle as 'settings.preferences.time_format.title' };
export { settingsPreferencesTimeFormatBrowserDefaultLabel as 'settings.preferences.time_format.browser_default.label' };
export { settingsPreferencesTimeFormatBrowserDefaultDescription as 'settings.preferences.time_format.browser_default.description' };
export { settingsPreferencesTimeFormat12hLabel as 'settings.preferences.time_format.12h.label' };
export { settingsPreferencesTimeFormat12hDescription as 'settings.preferences.time_format.12h.description' };
export { settingsPreferencesTimeFormat24hLabel as 'settings.preferences.time_format.24h.label' };
export { settingsPreferencesTimeFormat24hDescription as 'settings.preferences.time_format.24h.description' };
export { settingsPreferencesSaveButton as 'settings.preferences.save_button' };
export { settingsPreferencesSaved as 'settings.preferences.saved' };
export { settingsPreferencesSaveFailed as 'settings.preferences.save_failed' };
export { settingsNotificationsTitle as 'settings.notifications.title' };
export { settingsNotificationsSubtitle as 'settings.notifications.subtitle' };
export { settingsNotificationsPushTitle as 'settings.notifications.push.title' };
export { settingsNotificationsPushNotConfigured as 'settings.notifications.push.not_configured' };
export { settingsNotificationsPushNotSupported as 'settings.notifications.push.not_supported' };
export { settingsNotificationsPushBlockedTitle as 'settings.notifications.push.blocked_title' };
export { settingsNotificationsPushBlockedDescription as 'settings.notifications.push.blocked_description' };
export { settingsNotificationsPushEnabledTitle as 'settings.notifications.push.enabled_title' };
export { settingsNotificationsPushEnabledDescription as 'settings.notifications.push.enabled_description' };
export { settingsNotificationsPushEnableTitle as 'settings.notifications.push.enable_title' };
export { settingsNotificationsPushEnableDescription as 'settings.notifications.push.enable_description' };
export { settingsNotificationsPushEnableButton as 'settings.notifications.push.enable_button' };
export { settingsNotificationsPushEnabling as 'settings.notifications.push.enabling' };
export { settingsNotificationsPushBlockedError as 'settings.notifications.push.blocked_error' };
export { settingsNotificationsPushEnableFailed as 'settings.notifications.push.enable_failed' };
export { settingsNotificationsPushEnableError as 'settings.notifications.push.enable_error' };
export { settingsNotificationsLevelsLoading as 'settings.notifications.levels.loading' };
export { settingsNotificationsLevelsLoadFailed as 'settings.notifications.levels.load_failed' };
export { settingsNotificationsLevelsServerUpdated as 'settings.notifications.levels.server_updated' };
export { settingsNotificationsLevelsRoomUpdated as 'settings.notifications.levels.room_updated' };
export { settingsNotificationsLevelsUpdateFailed as 'settings.notifications.levels.update_failed' };
export { settingsNotificationsLevelsServerTitle as 'settings.notifications.levels.server_title' };
export { settingsNotificationsLevelsServerDescription as 'settings.notifications.levels.server_description' };
export { settingsNotificationsLevelsRoomTitle as 'settings.notifications.levels.room_title' };
export { settingsNotificationsLevelsRoomDescription as 'settings.notifications.levels.room_description' };
export { settingsNotificationsLevelsEffective as 'settings.notifications.levels.effective' };
export { settingsNotificationsLevelsDefaultLabel as 'settings.notifications.levels.default.label' };
export { settingsNotificationsLevelsDefaultDescription as 'settings.notifications.levels.default.description' };
export { settingsNotificationsLevelsMutedLabel as 'settings.notifications.levels.muted.label' };
export { settingsNotificationsLevelsMutedDescription as 'settings.notifications.levels.muted.description' };
export { settingsNotificationsLevelsNormalLabel as 'settings.notifications.levels.normal.label' };
export { settingsNotificationsLevelsNormalDescription as 'settings.notifications.levels.normal.description' };
export { settingsNotificationsLevelsAllMessagesLabel as 'settings.notifications.levels.all_messages.label' };
export { settingsNotificationsLevelsAllMessagesDescription as 'settings.notifications.levels.all_messages.description' };
export { settingsNotificationsSoundTitle as 'settings.notifications.sound.title' };
export { settingsNotificationsSoundShapeTitle as 'settings.notifications.sound.shape_title' };
export { settingsNotificationsSoundPreview as 'settings.notifications.sound.preview' };
export { settingsNotificationsSoundReset as 'settings.notifications.sound.reset' };
export { settingsNotificationsSoundOff as 'settings.notifications.sound.off' };
export { settingsNotificationsSoundVolume as 'settings.notifications.sound.volume' };
export { settingsNotificationsSoundTinny as 'settings.notifications.sound.tinny' };
export { settingsNotificationsSoundMuffled as 'settings.notifications.sound.muffled' };
export { settingsNotificationsSoundEcho as 'settings.notifications.sound.echo' };
export { settingsNotificationsSoundReverb as 'settings.notifications.sound.reverb' };
export { settingsNotificationsSoundCrunch as 'settings.notifications.sound.crunch' };
export { settingsNotificationsSoundCategorySilent as 'settings.notifications.sound.category.silent' };
export { settingsNotificationsSoundCategorySimple as 'settings.notifications.sound.category.simple' };
export { settingsNotificationsSoundCategoryPlayful as 'settings.notifications.sound.category.playful' };
export { settingsNotificationsSoundCategoryRobots as 'settings.notifications.sound.category.robots' };
export { settingsNotificationsSoundCategoryMusical as 'settings.notifications.sound.category.musical' };
export { settingsNotificationsSoundCategoryHereBeDragons as 'settings.notifications.sound.category.here_be_dragons' };
export { settingsNotificationsSoundNameSilent as 'settings.notifications.sound.name.silent' };
export { settingsNotificationsSoundNameDing as 'settings.notifications.sound.name.ding' };
export { settingsNotificationsSoundNameChimeUp as 'settings.notifications.sound.name.chime_up' };
export { settingsNotificationsSoundNameChimeDown as 'settings.notifications.sound.name.chime_down' };
export { settingsNotificationsSoundNamePop as 'settings.notifications.sound.name.pop' };
export { settingsNotificationsSoundNameBubble as 'settings.notifications.sound.name.bubble' };
export { settingsNotificationsSoundNameRetro as 'settings.notifications.sound.name.retro' };
export { settingsNotificationsSoundNameCoin as 'settings.notifications.sound.name.coin' };
export { settingsNotificationsSoundNamePowerup as 'settings.notifications.sound.name.powerup' };
export { settingsNotificationsSoundNameFanfare as 'settings.notifications.sound.name.fanfare' };
export { settingsNotificationsSoundNameLaser as 'settings.notifications.sound.name.laser' };
export { settingsNotificationsSoundNameRobot as 'settings.notifications.sound.name.robot' };
export { settingsNotificationsSoundNameUfo as 'settings.notifications.sound.name.ufo' };
export { settingsNotificationsSoundNameBeepboop as 'settings.notifications.sound.name.beepboop' };
export { settingsNotificationsSoundNameDialup as 'settings.notifications.sound.name.dialup' };
export { settingsNotificationsSoundNameR2d2 as 'settings.notifications.sound.name.r2d2' };
export { settingsNotificationsSoundNameHarp as 'settings.notifications.sound.name.harp' };
export { settingsNotificationsSoundNameMusicBox as 'settings.notifications.sound.name.music_box' };
export { settingsNotificationsSoundNameCelesta as 'settings.notifications.sound.name.celesta' };
export { settingsNotificationsSoundNameSynth as 'settings.notifications.sound.name.synth' };
export { settingsNotificationsSoundNameOrchestra as 'settings.notifications.sound.name.orchestra' };
export { settingsNotificationsSoundNameLaCucaracha as 'settings.notifications.sound.name.la_cucaracha' };
export { settingsNotificationsSoundNameChaos as 'settings.notifications.sound.name.chaos' };
export { settingsNotificationsSoundNameGlitch as 'settings.notifications.sound.name.glitch' };
export { settingsNotificationsSoundNameSiren as 'settings.notifications.sound.name.siren' };
export { settingsNotificationsSoundNameDubstep as 'settings.notifications.sound.name.dubstep' };
export { settingsNotificationsSoundNameCircus as 'settings.notifications.sound.name.circus' };
export { settingsAccountTitle as 'settings.account.title' };
export { settingsAccountSubtitle as 'settings.account.subtitle' };
export { settingsAccountInfoTitle as 'settings.account.info_title' };
export { settingsAccountUsername as 'settings.account.username' };
export { settingsAccountDisplayName as 'settings.account.display_name' };
export { settingsAccountDangerTitle as 'settings.account.danger_title' };
export { settingsAccountDangerDescription as 'settings.account.danger_description' };
export { settingsAccountDeleteButton as 'settings.account.delete_button' };
export { settingsAccountDeleteFailed as 'settings.account.delete_failed' };
export { settingsAccountDeleteRequestFailed as 'settings.account.delete_request_failed' };
export { settingsAccountDeleteModalTitle as 'settings.account.delete_modal.title' };
export { settingsAccountDeleteModalWarningLabel as 'settings.account.delete_modal.warning_label' };
export { settingsAccountDeleteModalWarningText as 'settings.account.delete_modal.warning_text' };
export { settingsAccountDeleteModalIntro as 'settings.account.delete_modal.intro' };
export { settingsAccountDeleteModalRemoveFromRooms as 'settings.account.delete_modal.remove_from_rooms' };
export { settingsAccountDeleteModalDeleteMessages as 'settings.account.delete_modal.delete_messages' };
export { settingsAccountDeleteModalDeleteProfile as 'settings.account.delete_modal.delete_profile' };
export { settingsAccountDeleteModalConfirmLabel as 'settings.account.delete_modal.confirm_label' };
export { settingsAccountDeleteModalConfirmPlaceholder as 'settings.account.delete_modal.confirm_placeholder' };
export { settingsAccountDeleteModalCancel as 'settings.account.delete_modal.cancel' };
export { settingsAccountDeleteModalDeleting as 'settings.account.delete_modal.deleting' };
