/// <reference types="@vitest/browser/matchers" />
/// <reference types="@vitest/browser/providers/playwright" />

import '@chatto/client/svelte';
import { afterEach } from 'vitest';
import { queryClient } from '$lib/query/client';

afterEach(() => {
  queryClient.clear();
});
