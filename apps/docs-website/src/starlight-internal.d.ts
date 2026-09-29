// DocsSiteTitle copies Starlight's SiteTitle, which reads the logo images from
// an internal virtual module. Starlight does not export types for it; this
// declaration matches @astrojs/starlight/virtual-internal.d.ts.
declare module 'virtual:starlight/user-images' {
  type ImageMetadata = import('astro').ImageMetadata;
  export const logos: {
    dark?: ImageMetadata;
    light?: ImageMetadata;
  };
}
