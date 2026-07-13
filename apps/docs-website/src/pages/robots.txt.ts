import type { APIRoute } from "astro";
import { docsChannel, docsSiteUrl } from "../docsMetadata";

export const GET: APIRoute = () => {
  const body =
    docsChannel === "stable"
      ? `User-agent: *\nAllow: /\nSitemap: ${docsSiteUrl}/sitemap-index.xml\n`
      : "User-agent: *\nDisallow: /\n";

  return new Response(body, {
    headers: { "Content-Type": "text/plain; charset=utf-8" },
  });
};
