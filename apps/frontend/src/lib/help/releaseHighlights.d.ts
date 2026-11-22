declare module 'virtual:chatto-release-highlights' {
  /** The highlights of each release in the root `CHANGELOG.md`, latest first, as Markdown. */
  const releases: { version: string; highlights: string[] }[];
  export default releases;
}
