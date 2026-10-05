/**
 * Localized labels for the two 12-hour clock periods, such as "AM" and "PM".
 *
 * This module has no other dependencies, so form controls in the initial
 * bundle can use it without loading the time formatting helpers.
 */
export function dayPeriodLabels(locale: string): { am: string; pm: string } {
  const formatter = new Intl.DateTimeFormat(locale, {
    hour: 'numeric',
    hour12: true,
    timeZone: 'UTC'
  });
  const label = (hour: number) =>
    formatter
      .formatToParts(new Date(Date.UTC(2000, 0, 1, hour)))
      .find((part) => part.type === 'dayPeriod')?.value;
  return { am: label(1) ?? 'AM', pm: label(13) ?? 'PM' };
}
