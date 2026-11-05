<script module lang="ts">
  /** Small built-in line icons; no network icon service is used. */
  export type IconName =
    | 'close'
    | 'back'
    | 'hash'
    | 'search'
    | 'send'
    | 'logout'
    | 'chevron'
    | 'chevronRight'
    | 'message'
    | 'check'
    | 'gallery'
    | 'plus'
    | 'settings';
  const paths: Record<IconName, string> = {
    close: '<path d="m6 6 12 12M18 6 6 18"/>',
    back: '<path d="m12 5-7 7 7 7M5 12h15"/>',
    plus: '<path d="M12 5v14M5 12h14"/>',
    settings:
      '<circle cx="12" cy="12" r="3"/><path d="M12 3v3m0 12v3M3 12h3m12 0h3M5.6 5.6l2.1 2.1m8.6 8.6 2.1 2.1m0-12.8-2.1 2.1m-8.6 8.6-2.1 2.1"/>',
    hash: '<path d="M10 3 6 21M18 3l-4 18M3 9h18M2 15h18"/>',
    search: '<circle cx="10.5" cy="10.5" r="6.5"/><path d="m16 16 5 5"/>',
    send: '<path d="m3 3 19 9-19 9 4-9-4-9Zm4 9h15"/>',
    logout: '<path d="M10 4H4v16h6m4-12 4 4-4 4m-6-4h13"/>',
    chevron: '<path d="m6 9 6 6 6-6"/>',
    chevronRight: '<path d="m9 6 6 6-6 6"/>',
    message: '<path d="M4 4h16v13H9l-5 4V4Zm4 5h8M8 12h6"/>',
    check: '<path d="m5 12 4 4L19 6"/>',
    gallery:
      '<rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><rect x="14" y="14" width="7" height="7" rx="1"/>'
  };
</script>

<script lang="ts">
  import { scaled } from './scale.svelte.ts';
  import { colors } from './theme.ts';
  let {
    name,
    size = 18,
    accent = false
  }: { name: IconName; size?: number; accent?: boolean } = $props();
  const source = $derived(
    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="${accent ? colors.action : colors.muted}" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">${paths[name]}</svg>`
  );
</script>

<svg
  {source}
  style={`width: ${scaled(size)}px; height: ${scaled(size)}px; flex-shrink: 0; color: ${accent ? colors.action : colors.muted};`}
/>
