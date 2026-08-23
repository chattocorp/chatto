<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import { localeDisplayName, selectableLocales } from '$lib/i18n/locales';
  import { getLocale, setLocale, type Locale } from '$lib/i18n/runtime';
  import { ChoiceRow, FormSection, PageTitle, PaneHeader } from '$lib/ui';

  const activeLocale = $derived(getLocale());
  const languageOptions = $derived(
    selectableLocales.map((locale) => ({
      value: locale,
      label: localeDisplayName(locale, activeLocale)
    }))
  );

  function handleLocaleSelect(locale: Locale) {
    if (locale === activeLocale) return;
    void setLocale(locale);
  }
</script>

<PageTitle title={m('settings.preferences.language.title')} />
<PaneHeader
  title={m('settings.preferences.language.title')}
  subtitle={m('settings.preferences.language.description')}
/>

<div class="flex flex-col gap-6 overflow-y-auto p-6">
  <FormSection title={m('settings.preferences.language.title')} maxWidth="max-w-md">
    <div
      class="flex flex-col gap-2"
      role="radiogroup"
      aria-label={m('settings.preferences.language.title')}
    >
      {#each languageOptions as option (option.value)}
        <ChoiceRow
          label={option.label}
          selected={activeLocale === option.value}
          onclick={() => handleLocaleSelect(option.value)}
        />
      {/each}
    </div>
  </FormSection>
</div>
