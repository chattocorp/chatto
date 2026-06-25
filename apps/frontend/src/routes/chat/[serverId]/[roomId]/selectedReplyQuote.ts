import type { QuoteInsertionContent, SelectedQuoteBlock } from '$lib/state/room';

export function normalizeSelectedQuoteText(text: string): string | null {
  const normalized = text.replace(/\r\n?/g, '\n').trim();
  return normalized ? normalized : null;
}

function nodeIsInside(root: HTMLElement, node: Node | null): boolean {
  if (!node) return false;
  return root === node || root.contains(node);
}

function countBlockquoteAncestors(root: HTMLElement, node: Node): number {
  let depth = 0;
  let current: Node | null = node.parentNode;

  while (current && current !== root) {
    if (current instanceof HTMLElement && current.tagName === 'BLOCKQUOTE') {
      depth++;
    }
    current = current.parentNode;
  }

  return depth;
}

function selectedTextForBlock(range: Range, block: HTMLElement): string | null {
  let text = '';
  const walker = document.createTreeWalker(block, NodeFilter.SHOW_TEXT | NodeFilter.SHOW_ELEMENT, {
    acceptNode(node) {
      if (node instanceof HTMLElement && node.tagName !== 'BR') {
        return NodeFilter.FILTER_SKIP;
      }
      return range.intersectsNode(node) ? NodeFilter.FILTER_ACCEPT : NodeFilter.FILTER_REJECT;
    }
  });

  while (walker.nextNode()) {
    const node = walker.currentNode;
    if (node instanceof Text) {
      const start = range.startContainer === node ? range.startOffset : 0;
      const end = range.endContainer === node ? range.endOffset : node.data.length;
      if (start < end) text += node.data.slice(start, end);
    } else if (node instanceof HTMLElement && node.tagName === 'BR') {
      text += '\n';
    }
  }

  return normalizeSelectedQuoteText(text);
}

function selectedQuoteBlocksForRange(
  range: Range,
  messageBodyRoot: HTMLElement
): SelectedQuoteBlock[] {
  const blocks: SelectedQuoteBlock[] = [];
  const blockSelector = 'p, li, pre, h1, h2, h3, h4, h5, h6';
  const walker = document.createTreeWalker(messageBodyRoot, NodeFilter.SHOW_ELEMENT, {
    acceptNode(node) {
      if (!(node instanceof HTMLElement) || !node.matches(blockSelector)) {
        return NodeFilter.FILTER_SKIP;
      }
      return range.intersectsNode(node) ? NodeFilter.FILTER_ACCEPT : NodeFilter.FILTER_REJECT;
    }
  });

  while (walker.nextNode()) {
    const block = walker.currentNode as HTMLElement;
    const text = selectedTextForBlock(range, block);
    if (text) {
      blocks.push({
        quoteDepth: countBlockquoteAncestors(messageBodyRoot, block),
        text
      });
    }
  }

  return blocks;
}

export function selectedQuoteTextForMessageBody(
  selection: Selection | null,
  messageBodyRoot: HTMLElement | null | undefined
): QuoteInsertionContent | null {
  if (!selection || selection.isCollapsed || selection.rangeCount === 0 || !messageBodyRoot) {
    return null;
  }

  if (
    !nodeIsInside(messageBodyRoot, selection.anchorNode) ||
    !nodeIsInside(messageBodyRoot, selection.focusNode)
  ) {
    return null;
  }

  const range = selection.getRangeAt(0);
  const blocks = selectedQuoteBlocksForRange(range, messageBodyRoot);
  return blocks.length > 0 ? blocks : normalizeSelectedQuoteText(selection.toString());
}
