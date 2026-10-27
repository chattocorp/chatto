/** Add frame and inset surfaces to Pele's generated default flowchart groups. */
export function frameMermaidGroups(svg: string): string {
  return svg.replace(
    /(<g class="pele-cluster"[^>]*>)(<rect\b[^>]*\/>)/gu,
    (match, group: string, rect: string) => {
      // Author-defined fills keep their own presentation. This matches only the
      // default rectangle emitted by Pele, never user-supplied HTML.
      if (!rect.includes('fill="var(--_a)"')) return match;
      const style = rect.match(/\bstyle="([^"]*)"/u)?.[1];
      if (style !== 'rx:var(--_r)') return match;
      const coordinate = (name: string) =>
        Number(rect.match(new RegExp(`\\b${name}="([^"]+)"`))?.[1]);
      const x = coordinate('x');
      const y = coordinate('y');
      const width = coordinate('width');
      const height = coordinate('height');
      if (![x, y, width, height].every(Number.isFinite) || width < 16 || height < 48) return match;
      // Pele reserves the upper band for its group title. Keep that band part of
      // the frame, with the same four-pixel inset used by Chatto's menu sections.
      return (
        `${group}${rect.replace('<rect', '<rect class="chatto-pele-frame"').replace('rx:var(--_r)', 'rx:8px')}` +
        `<rect class="chatto-pele-frame-shade" x="${x}" y="${y}" width="${width}" height="${height}" rx="8" fill="none"/>` +
        `<rect class="chatto-pele-inset" x="${x + 4}" y="${y + 36}" width="${width - 8}" height="${height - 40}" rx="6" fill="var(--pele-bg)"/>`
      );
    }
  );
}
