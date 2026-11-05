/** Static component imports are compiled by gpuix-svelte's loader or Bun plugin. */
declare module '*.svelte' {
  const component: import('gpuix-svelte').AnyComponent;
  export default component;
}
