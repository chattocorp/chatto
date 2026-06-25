import type { Locator } from '@playwright/test';
import * as fs from 'fs/promises';
import * as path from 'path';

/**
 * Simulate a dragenter event on a locator.
 */
export async function simulateDragEnter(locator: Locator): Promise<void> {
  await locator.evaluate((el) => {
    const dataTransfer = new DataTransfer();
    dataTransfer.items.add(new File([''], 'test.png', { type: 'image/png' }));

    el.dispatchEvent(
      new DragEvent('dragenter', {
        bubbles: true,
        cancelable: true,
        dataTransfer
      })
    );
  });
}

/**
 * Simulate a dragleave event on a locator.
 */
export async function simulateDragLeave(locator: Locator): Promise<void> {
  await locator.evaluate((el) => {
    const dataTransfer = new DataTransfer();
    dataTransfer.items.add(new File([''], 'test.png', { type: 'image/png' }));

    el.dispatchEvent(
      new DragEvent('dragleave', {
        bubbles: true,
        cancelable: true,
        dataTransfer,
        relatedTarget: null
      })
    );
  });
}

const MIME_TYPES: Record<string, string> = {
  '.png': 'image/png',
  '.jpg': 'image/jpeg',
  '.jpeg': 'image/jpeg',
  '.gif': 'image/gif',
  '.webp': 'image/webp'
};

/**
 * Simulate dropping a file onto a locator.
 */
export async function simulateFileDrop(locator: Locator, filePath: string): Promise<void> {
  const absolutePath = path.resolve(filePath);
  const fileBuffer = await fs.readFile(absolutePath);
  const fileName = path.basename(absolutePath);
  const base64 = fileBuffer.toString('base64');

  const ext = path.extname(fileName).toLowerCase();
  const mimeType = MIME_TYPES[ext] || 'application/octet-stream';

  await locator.evaluate(
    (el, { base64Data, fileName, mimeType }) => {
      const binaryString = atob(base64Data);
      const bytes = new Uint8Array(binaryString.length);
      for (let i = 0; i < binaryString.length; i++) {
        bytes[i] = binaryString.charCodeAt(i);
      }

      const file = new File([bytes], fileName, { type: mimeType });
      const dataTransfer = new DataTransfer();
      dataTransfer.items.add(file);

      el.dispatchEvent(
        new DragEvent('drop', {
          bubbles: true,
          cancelable: true,
          dataTransfer
        })
      );
    },
    { base64Data: base64, fileName, mimeType }
  );
}
