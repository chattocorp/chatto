/**
 * Identify Markdown for the frontend viewer. A filename is used only when the
 * uploader supplied no specific document type.
 */
export function isMarkdownAttachment(contentType: string, filename: string): boolean {
  const type = contentType.split(';', 1)[0].trim().toLowerCase();
  if (type === 'text/markdown' || type === 'text/x-markdown') return true;
  return (
    (type === '' || type === 'text/plain' || type === 'application/octet-stream') &&
    /\.(md|markdown)$/i.test(filename)
  );
}
