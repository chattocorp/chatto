import { defineRouteMiddleware } from "@astrojs/starlight/route-data";
import type { StarlightRouteData } from "@astrojs/starlight/route-data";

type Head = StarlightRouteData["head"];

const upsertMeta = (
  head: Head,
  attribute: "name" | "property",
  key: string,
  content: string,
) => {
  const entry = {
    tag: "meta" as const,
    attrs: { [attribute]: key, content },
  };
  const index = head.findIndex(
    ({ tag, attrs }) => tag === "meta" && attrs?.[attribute] === key,
  );

  if (index === -1) {
    head.push(entry);
  } else {
    head[index] = entry;
  }
};

export const onRequest = defineRouteMiddleware(({ locals, site }) => {
  if (!site) throw new Error("Astro's site URL is required for Open Graph metadata.");

  const route = locals.starlightRoute;
  if (route.id === "404") return;

  const title = route.entry.data.title;
  const description =
    route.entry.data.description ?? "Self-hosting documentation for Chatto.";
  const imageSlug = route.id || "index";
  const imageUrl = new URL(`/open-graph/${imageSlug}.png`, site).href;
  const imageAlt = `Preview of ${title} in the Chatto documentation.`;

  upsertMeta(route.head, "property", "og:type", route.id ? "article" : "website");
  upsertMeta(route.head, "property", "og:image", imageUrl);
  upsertMeta(route.head, "property", "og:image:width", "1200");
  upsertMeta(route.head, "property", "og:image:height", "630");
  upsertMeta(route.head, "property", "og:image:alt", imageAlt);
  upsertMeta(route.head, "name", "twitter:title", title);
  upsertMeta(route.head, "name", "twitter:description", description);
  upsertMeta(route.head, "name", "twitter:image", imageUrl);
  upsertMeta(route.head, "name", "twitter:image:alt", imageAlt);
});
