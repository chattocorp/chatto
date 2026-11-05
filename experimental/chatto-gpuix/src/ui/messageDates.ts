/** Local calendar labels for the native timeline; formatters are shared by all rows. */
const clock = new Intl.DateTimeFormat(undefined, { hour: '2-digit', minute: '2-digit' });
const calendar = new Intl.DateTimeFormat(undefined, {
  weekday: 'long',
  month: 'long',
  day: 'numeric',
  year: 'numeric'
});

/** A local day key for deciding when the timeline needs a date divider. */
export function messageDay(value: string): string {
  return new Date(value).toDateString();
}
/** Show the date at a day boundary instead of repeating an ISO timestamp on each message. */
export function messageDate(value: string): string {
  return calendar.format(new Date(value));
}
/** Format one message timestamp in the system locale and timezone. */
export function messageTime(value: string): string {
  return clock.format(new Date(value));
}
