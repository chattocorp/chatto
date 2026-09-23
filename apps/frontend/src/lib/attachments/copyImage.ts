/** Copy the displayed attachment image as PNG without exposing its signed URL. */
export async function copyImageToClipboard(url: string): Promise<void> {
  // Start the clipboard write during the menu click. Some browsers require
  // transient user activation before the image fetch and conversion finish.
  await navigator.clipboard.write([
    new ClipboardItem({ 'image/png': imagePng(url) })
  ]);
}

async function imagePng(url: string): Promise<Blob> {
  const response = await fetch(url, { credentials: 'omit', referrerPolicy: 'no-referrer' });
  if (!response.ok) throw new Error('Image request failed');

  const bitmap = await createImageBitmap(await response.blob());
  try {
    const canvas = document.createElement('canvas');
    canvas.width = bitmap.width;
    canvas.height = bitmap.height;
    const context = canvas.getContext('2d');
    if (!context) throw new Error('Image canvas unavailable');
    context.drawImage(bitmap, 0, 0);
    return await new Promise<Blob>((resolve, reject) => {
      canvas.toBlob((blob) => {
        if (blob) resolve(blob);
        else reject(new Error('Image conversion failed'));
      }, 'image/png');
    });
  } finally {
    bitmap.close();
  }
}
