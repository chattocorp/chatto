/** Presentation hints supported by the bundled Markdown renderer and composers. */
export type ArtifactFence = {
  language: 'svg' | 'mermaid' | 'ansi';
  presentation: 'code' | 'visual';
};

/** Accept only a complete artifact language with the optional `rendered` hint. */
export function parseArtifactFence(info: string | null | undefined): ArtifactFence | null {
  const match = /^(svg|mermaid|ansi)(\s+rendered)?$/i.exec(info?.trim() ?? '');
  if (!match) return null;
  return {
    language: match[1].toLowerCase() as ArtifactFence['language'],
    presentation: match[2] ? 'visual' : 'code'
  };
}

/** Preserve visual presentation only when the selected language supports it. */
export function formatArtifactFence(language: string, visual: boolean): string {
  const artifact = parseArtifactFence(language);
  return artifact ? `${artifact.language}${visual ? ' rendered' : ''}` : language;
}
