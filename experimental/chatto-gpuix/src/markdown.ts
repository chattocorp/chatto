/** Normalize message Markdown before native rendering; remote media never loads. */
import { fromMarkdown } from 'mdast-util-from-markdown';
import { toMarkdown } from 'mdast-util-to-markdown';
import { map } from 'unist-util-map';

/** Images become labels, HTML becomes code, and links become text emphasis. */
export function safeMarkdown(source: string): string {
  const tree = map(fromMarkdown(source), (node, _index, parent) => {
    if (node.type === 'image' || node.type === 'imageReference') {
      return { type: 'text' as const, value: `[Image: ${node.alt || 'attachment'}]` };
    }
    if (node.type === 'html') {
      const inline =
        parent &&
        ['paragraph', 'heading', 'emphasis', 'strong', 'link', 'linkReference'].includes(
          parent.type
        );
      return inline
        ? { type: 'inlineCode' as const, value: node.value }
        : { type: 'code' as const, value: node.value };
    }
    if (node.type === 'link' || node.type === 'linkReference')
      return { type: 'emphasis' as const, children: node.children };
    return node;
  });
  return toMarkdown(tree);
}
