export function normalizeSelectedQuoteText(text: string): string | null {
  const normalized = text.replace(/\r\n?/g, '\n').trim();
  return normalized ? normalized : null;
}

function nodeIsInside(root: HTMLElement, node: Node | null): boolean {
  if (!node) return false;
  return root === node || root.contains(node);
}

export function selectedQuoteTextForMessageBody(
  selection: Selection | null,
  messageBodyRoot: HTMLElement | null | undefined
): string | null {
  if (!selection || selection.isCollapsed || selection.rangeCount === 0 || !messageBodyRoot) {
    return null;
  }

  if (
    !nodeIsInside(messageBodyRoot, selection.anchorNode) ||
    !nodeIsInside(messageBodyRoot, selection.focusNode)
  ) {
    return null;
  }

  return normalizeSelectedQuoteText(selection.toString());
}
