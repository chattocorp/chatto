import { serveDir } from "@std/http/file-server";
import { fileURLToPath } from "node:url";

const FRONTEND_ROOT: string = fileURLToPath(
  new URL("../frontend/build/", import.meta.url),
);

/** Serve the official frontend assets with their SPA navigation fallback. */
export function createFrontendHandler(
  fsRoot = FRONTEND_ROOT,
): (request: Request) => Promise<Response> {
  return async (request) => {
    const response = await serveDir(request, { fsRoot, quiet: true });
    if (!shouldUseSpaFallback(request, response)) return response;

    const fallback = new Request(new URL("/200.html", request.url), {
      headers: request.headers,
    });
    return await serveDir(fallback, { fsRoot, quiet: true });
  };
}

function shouldUseSpaFallback(request: Request, response: Response): boolean {
  if (response.status !== 404 || request.method !== "GET") return false;

  const url = new URL(request.url);
  return url.pathname === "/" ||
    request.headers.get("sec-fetch-dest") === "document" ||
    (request.headers.get("accept") ?? "").includes("text/html");
}
