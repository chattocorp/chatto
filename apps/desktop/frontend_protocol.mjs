import { stat } from "node:fs/promises";
import path from "node:path";
import { pathToFileURL } from "node:url";

export const APP_ORIGIN = "chatto://desktop";

/**
 * Create the custom protocol handler that gives the embedded frontend a
 * stable, secure browser origin.
 */
export function createFrontendProtocolHandler(frontendRoot, fetchResource) {
  const root = path.resolve(frontendRoot);

  return async (request) => {
    const url = new URL(request.url);
    if (url.protocol !== "chatto:" || url.host !== "desktop") return notFound();

    if (request.method !== "GET" && request.method !== "HEAD")
      return notFound();

    const requestedPath = decodeURIComponent(url.pathname).replace(/^\/+/, "");
    const candidate = path.resolve(root, requestedPath || "200.html");
    if (!isWithin(root, candidate)) return notFound();

    let asset = candidate;
    if (!(await isFile(asset))) {
      if (!request.headers.get("accept")?.includes("text/html"))
        return notFound();
      asset = path.join(root, "200.html");
    }
    if (!(await isFile(asset))) return notFound();

    const response = await fetchResource(pathToFileURL(asset).toString());
    if (request.method !== "HEAD") return response;
    return new Response(null, {
      status: response.status,
      headers: response.headers,
    });
  };
}

function notFound() {
  return new Response("Not found", {
    status: 404,
    headers: { "content-type": "text/plain; charset=utf-8" },
  });
}

function isWithin(root, candidate) {
  const relative = path.relative(root, candidate);
  return (
    relative === "" ||
    (!relative.startsWith("..") && !path.isAbsolute(relative))
  );
}

async function isFile(candidate) {
  try {
    return (await stat(candidate)).isFile();
  } catch {
    return false;
  }
}
