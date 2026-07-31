import type { MessageSearchInput } from '$lib/api-client/messageSearch';
import type { MessageSearchStore } from '$lib/state/server/messageSearch.svelte';
import { useDebounce } from './useDebounce.svelte';

type SearchInput = Omit<MessageSearchInput, 'cursor'>;

type DebouncedMessageSearchOptions = {
  getStore: () => MessageSearchStore;
  getInput: (query: string) => SearchInput;
  delay?: number;
};

/** Coordinate normalized, debounced searches without replacing the raw input value. */
export function useDebouncedMessageSearch({
  getStore,
  getInput,
  delay = 300
}: DebouncedMessageSearchOptions) {
  const debounce = useDebounce();
  let activeStore: MessageSearchStore | null = null;
  let pendingKey: string | null = null;
  let submittedKey: string | null = null;
  const searchTerm = $derived.by(() => getStore().query.trim());

  function inputKey(input: SearchInput): string {
    return JSON.stringify(input);
  }

  function syncStore(store: MessageSearchStore): void {
    if (store === activeStore) return;
    debounce.cancel();
    activeStore = store;
    pendingKey = null;
    submittedKey = store.hasSearched ? inputKey(getInput(searchTerm)) : null;
  }

  function runSearch(
    store: MessageSearchStore,
    input: SearchInput,
    key: string,
    force = false
  ): void {
    if (getStore() !== store || activeStore !== store) return;
    pendingKey = null;
    if (!searchTerm || inputKey(getInput(searchTerm)) !== key) return;
    if (!store.available || (!force && key === submittedKey && store.hasSearched && !store.error)) {
      return;
    }
    submittedKey = key;
    void store.search(input, { preserveQuery: true });
  }

  function schedule(rawQuery: string): void {
    const store = getStore();
    syncStore(store);
    store.query = rawQuery;
    const query = searchTerm;
    if (!query) {
      debounce.cancel();
      pendingKey = null;
      submittedKey = null;
      store.clearResults();
      return;
    }

    const input = getInput(query);
    const key = inputKey(input);
    if (key === pendingKey || (key === submittedKey && store.hasSearched && !store.error)) return;

    debounce.cancel();
    pendingKey = key;
    submittedKey = null;
    store.prepareQueryChange();
    debounce.run(() => runSearch(store, input, key), delay);
  }

  function submitNow(): void {
    const store = getStore();
    syncStore(store);
    debounce.cancel();
    pendingKey = null;
    const query = searchTerm;
    if (!query || !store.available) return;
    const input = getInput(query);
    runSearch(store, input, inputKey(input), true);
  }

  return {
    schedule,
    submitNow,
    sync: () => syncStore(getStore())
  };
}
