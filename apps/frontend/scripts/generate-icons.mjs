import sharp from 'sharp';
import { mkdir } from 'fs/promises';
import { fileURLToPath } from 'url';
import { dirname, join } from 'path';

const __dirname = dirname(fileURLToPath(import.meta.url));
const srcDir = join(__dirname, '../src/lib/assets');
const outDir = join(__dirname, '../static/icons');

const sizes = [
  // Regular icons
  { name: 'icon-192.png', size: 192, source: 'chatto-icon.png' },
  { name: 'icon-512.png', size: 512, source: 'chatto-icon.png' },
  { name: 'apple-touch-icon.png', size: 180, source: 'chatto-icon.png' },
  { name: 'favicon.png', size: 32, source: 'chatto-icon.png' },
  // Maskable icons (for Android adaptive icons)
  { name: 'icon-maskable-192.png', size: 192, source: 'chatto-icon-maskable.png' },
  { name: 'icon-maskable-512.png', size: 512, source: 'chatto-icon-maskable.png' }
];

async function main() {
  await mkdir(outDir, { recursive: true });

  for (const { name, size, source } of sizes) {
    const sourcePath = join(srcDir, source);
    await sharp(sourcePath).resize(size, size).png().toFile(join(outDir, name));
    console.log(`Generated ${name} (${size}x${size}) from ${source}`);
  }
}

main().catch(console.error);
