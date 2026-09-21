/** Submit through the normal validation and submit-event path. */
export function submitFormShortcut(event: KeyboardEvent, pending = false): void {
  if (
    event.defaultPrevented || event.isComposing || event.repeat ||
    event.key !== "Enter" || !(event.metaKey || event.ctrlKey) ||
    event.altKey || event.shiftKey
  ) return;
  event.preventDefault();
  if (!pending) (event.currentTarget as HTMLFormElement).requestSubmit();
}
