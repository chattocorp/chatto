import sharp from 'sharp';
import { mkdir } from 'fs/promises';
import { fileURLToPath } from 'url';
import { dirname, join } from 'path';

const __dirname = dirname(fileURLToPath(import.meta.url));
const srcDir = join(__dirname, '../src/lib/assets');
const outDir = join(__dirname, '../static/icons');
const installIconBackground = '#c5a4d4';

const sizes = [
  // Install-facing icons must be full-bleed and opaque so Safari/macOS does
  // not wrap a transparent, already-rounded icon in another app tile.
  { name: 'icon-192.png', size: 192, source: 'chatto-icon-maskable.png', flatten: true },
  { name: 'icon-512.png', size: 512, source: 'chatto-icon-maskable.png', flatten: true },
  { name: 'apple-touch-icon.png', size: 180, source: 'chatto-icon-maskable.png', flatten: true },
  { name: 'icon-maskable-192.png', size: 192, source: 'chatto-icon-maskable.png', flatten: true },
  { name: 'icon-maskable-512.png', size: 512, source: 'chatto-icon-maskable.png', flatten: true },
  // Browser tab favicon keeps the smaller rounded composition.
  { name: 'favicon.png', size: 32, source: 'chatto-icon.png', flatten: false },
  // Android masks the small notification badge. Keep its background and
  // facial details transparent instead of using an opaque install icon.
  {
    name: 'notification-badge.png',
    size: 96,
    source: 'chatto-notification-badge.svg',
    flatten: false
  }
];

async function main() {
  await mkdir(outDir, { recursive: true });

  for (const { name, size, source, flatten } of sizes) {
    const sourcePath = join(srcDir, source);
    let image = sharp(sourcePath).resize(size, size);
    if (flatten) {
      image = image.flatten({ background: installIconBackground });
    }
    await image.png().toFile(join(outDir, name));
    console.log(`Generated ${name} (${size}x${size}) from ${source}`);
  }

  // Link previews (og:image) use a 1200x630 card: the install icon centred
  // on its own background colour. Deployments can replace it at startup.
  // The maskable source paints an opaque background of this colour.
  const previewBackground = '#c9a7d8';
  const preview = { name: 'social-preview.png', width: 1200, height: 630, iconSize: 420 };
  const icon = await sharp(join(srcDir, 'chatto-icon-maskable.png'))
    .resize(preview.iconSize, preview.iconSize)
    .toBuffer();
  await sharp({
    create: {
      width: preview.width,
      height: preview.height,
      channels: 3,
      background: previewBackground
    }
  })
    .composite([{ input: icon, gravity: 'center' }])
    .png()
    .toFile(join(outDir, preview.name));
  console.log(`Generated ${preview.name} (${preview.width}x${preview.height})`);
}

main().catch(console.error);
