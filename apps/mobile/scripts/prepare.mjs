import { access, cp, mkdir, rm } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import sharp from 'sharp';

// App Store icons must have no alpha channel, even when every pixel is opaque.
// Generate from the shared artwork each sync so replacement artwork stays valid.
const iconSource = new URL('../../frontend/src/lib/assets/chatto-icon-maskable.png', import.meta.url);
const iconDestination = new URL('../ios/App/App/Assets.xcassets/AppIcon.appiconset/AppIcon-512@2x.png', import.meta.url);
await sharp(fileURLToPath(iconSource))
  .resize(1024, 1024)
  .flatten({ background: '#c5a4d4' })
  .removeAlpha()
  .png()
  .toFile(fileURLToPath(iconDestination));
const icon = await sharp(fileURLToPath(iconDestination)).metadata();
if (icon.width !== 1024 || icon.height !== 1024 || icon.hasAlpha) {
  throw new Error('The iOS app icon must be 1024x1024 with no alpha channel.');
}

// Copy only build artifacts; the shared frontend source remains authoritative.
const source = new URL('../../frontend/build/', import.meta.url);
const destination = new URL('../www/', import.meta.url);
await access(new URL('200.html', source));
await rm(destination, { recursive: true, force: true });
await mkdir(destination, { recursive: true });
await cp(source, destination, { recursive: true });
// Capacitor serves index.html for SPA routes. Use SvelteKit's fallback shell.
await cp(new URL('200.html', source), new URL('index.html', destination));
await cp(new URL('../node_modules/@capacitor/core/LICENSE', import.meta.url),
  new URL('capacitor-license.txt', destination));
await cp(new URL('../node_modules/@capacitor/keyboard/LICENSE', import.meta.url),
  new URL('capacitor-keyboard-license.txt', destination));
