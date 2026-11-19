/**
 * Add stable anchors and a contents list to authored Markdown at build time.
 * Only top-level h2/h3 sections belong in the contents; embedded examples do not.
 * MDsveX exposes `file.data.fm` as the compiled component's metadata export.
 */
export function helpHeadings() {
  /**
   * @typedef {{type: string, value?: string, alt?: string, depth?: number,
   * children?: MarkdownNode[], data?: {hProperties?: Record<string, unknown>}}} MarkdownNode
   * @param {MarkdownNode} tree
   * @param {{data: {fm?: Record<string, unknown>}}} file
   */
  return (tree, file) => {
    const used = new Set();
    const toc = [];
    /** @param {MarkdownNode} node @returns {string} */
    function text(node) {
      return node.value ?? node.alt ?? node.children?.map(text).join('') ?? '';
    }
    for (const node of tree.children ?? []) {
      if (node.type !== 'heading' || (node.depth !== 2 && node.depth !== 3)) continue;
      const title = text(node);
      const slug = title
        .normalize('NFKC')
        .toLowerCase()
        .replace(/[^\p{L}\p{N}]+/gu, '-')
        .replace(/^-|-$/g, '');
      const base = `section-${slug || 'heading'}`;
      let id = base;
      let suffix = 2;
      while (used.has(id)) id = `${base}-${suffix++}`;
      used.add(id);
      node.data = { ...node.data, hProperties: { ...node.data?.hProperties, id, tabindex: -1 } };
      toc.push({ id, title, depth: node.depth });
    }
    file.data.fm = { ...file.data.fm, toc };
  };
}
