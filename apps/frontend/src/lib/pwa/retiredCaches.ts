/** Delete only caches owned by retired Chatto shell and badge implementations. */
export async function deleteRetiredCaches(): Promise<void> {
  if (typeof caches === 'undefined') return;
  const names = await caches.keys();
  await Promise.all(
    names
      .filter(
        (name) =>
          name.startsWith('chatto-shell-') ||
          name === 'chatto-badge-state-v1' ||
          name === 'chatto-badge-state-v2'
      )
      .map((name) => caches.delete(name))
  );
}
