import { createServer, type Server } from 'http';

export interface OGMockServer {
  port: number;
  baseURL: string;
  close: () => Promise<void>;
}

// Minimal valid 1x1 red PNG (68 bytes)
const TINY_PNG = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8/5+hHgAHggJ/PchI7wAAAABJRU5ErkJggg==',
  'base64'
);

function ogPage(opts: {
  title: string;
  description: string;
  siteName: string;
  imageUrl?: string;
}): string {
  const imageMeta = opts.imageUrl ? `<meta property="og:image" content="${opts.imageUrl}" />` : '';
  return `<!DOCTYPE html>
<html>
<head>
	<meta property="og:title" content="${opts.title}" />
	<meta property="og:description" content="${opts.description}" />
	<meta property="og:site_name" content="${opts.siteName}" />
	${imageMeta}
</head>
<body>Test page</body>
</html>`;
}

/**
 * Starts a lightweight HTTP server that serves pages with known OpenGraph metadata.
 * Used by e2e tests so the Chatto server can fetch link previews from localhost
 * instead of hitting the real internet.
 *
 * Routes:
 * - GET /og-basic       → OG page with title, description, site name (no image)
 * - GET /og-with-image  → OG page with title, description, site name, and og:image
 * - GET /test-image.png → A minimal valid PNG image
 */
export function startOGMockServer(): Promise<OGMockServer> {
  return new Promise((resolve, reject) => {
    const server: Server = createServer((req, res) => {
      const url = req.url ?? '/';

      if (url === '/og-basic') {
        const html = ogPage({
          title: 'Test Page Title',
          description: 'This is a test description for link preview testing.',
          siteName: 'Test Site'
        });
        res.writeHead(200, { 'Content-Type': 'text/html' });
        res.end(html);
      } else if (url === '/og-with-image') {
        const port = (server.address() as { port: number }).port;
        const html = ogPage({
          title: 'Test Page With Image',
          description: 'This page has an OG image.',
          siteName: 'Image Test Site',
          imageUrl: `http://127.0.0.1:${port}/test-image.png`
        });
        res.writeHead(200, { 'Content-Type': 'text/html' });
        res.end(html);
      } else if (url === '/og-second') {
        const html = ogPage({
          title: 'Second Page Title',
          description: 'This is the second test page.',
          siteName: 'Second Site'
        });
        res.writeHead(200, { 'Content-Type': 'text/html' });
        res.end(html);
      } else if (url === '/test-image.png') {
        res.writeHead(200, {
          'Content-Type': 'image/png',
          'Content-Length': String(TINY_PNG.length)
        });
        res.end(TINY_PNG);
      } else {
        res.writeHead(404, { 'Content-Type': 'text/plain' });
        res.end('Not Found');
      }
    });

    // Listen on all interfaces so both IPv4 (127.0.0.1) and IPv6 (::1) work.
    // Go's net.Dial may resolve "localhost" to ::1 on macOS.
    server.listen(0, () => {
      const addr = server.address();
      if (!addr || typeof addr === 'string') {
        reject(new Error('Failed to get server address'));
        return;
      }
      resolve({
        port: addr.port,
        // Use 127.0.0.1 instead of "localhost" to avoid DNS resolution
        // ambiguity between IPv4 and IPv6 in the Go SSRF dial context.
        baseURL: `http://127.0.0.1:${addr.port}`,
        close: () =>
          new Promise<void>((res, rej) => {
            server.close((err) => (err ? rej(err) : res()));
          })
      });
    });

    server.on('error', reject);
  });
}
