import { getCollection } from "astro:content";
import { OGImageRoute } from "astro-og-canvas";

const entries = await getCollection("docs", ({ data }) => data.draft === false);

const pages = Object.fromEntries(
  entries.map(({ id, data }) => [
    id,
    {
      title: data.title,
      description:
        data.description ?? "Self-hosting documentation for Chatto.",
    },
  ]),
);

const wrapText = (value: string, maxLineLength: number) => {
  const words = value.split(/\s+/);
  const lines: string[] = [];
  let line = "";

  for (const word of words) {
    const next = line ? `${line} ${word}` : word;
    if (line && next.length > maxLineLength) {
      lines.push(line);
      line = word;
    } else {
      line = next;
    }
  }

  if (line) lines.push(line);
  return lines.join("\n");
};

const formatTitle = (title: string) => {
  const spacedTitle = title.replace(/([a-z\d])([A-Z])/g, "$1 $2");
  const brandedTitle = spacedTitle.startsWith("Chatto")
    ? spacedTitle
    : `Chatto · ${spacedTitle}`;
  return wrapText(brandedTitle, 24);
};

const formatDescription = (description: string) => {
  const shortened = description.slice(0, 180);
  const lastSpace = shortened.lastIndexOf(" ");
  const breakAt = lastSpace > 0 ? lastSpace : shortened.length;
  const displayDescription =
    description.length > shortened.length
      ? `${shortened.slice(0, breakAt).trimEnd()}…`
      : shortened;
  return wrapText(displayDescription, 58);
};

export const { getStaticPaths, GET } = await OGImageRoute({
  pages,
  getImageOptions: (_path, page) => ({
    title: formatTitle(page.title),
    description: formatDescription(page.description),
    bgGradient: [
      [12, 15, 22],
      [20, 24, 32],
    ],
    logo: {
      path: "./src/assets/opengraph-logo.png",
      size: [64, 64],
    },
    padding: 72,
    fonts: [
      "./node_modules/@fontsource-variable/ibm-plex-sans/files/ibm-plex-sans-latin-wght-normal.woff2",
    ],
    font: {
      title: {
        color: [255, 255, 255],
        size: 72,
        weight: 600,
        lineHeight: 1.05,
        families: ["IBM Plex Sans"],
      },
      description: {
        color: [196, 201, 210],
        size: 34,
        weight: 400,
        lineHeight: 1.3,
        families: ["IBM Plex Sans"],
      },
    },
    format: "PNG",
  }),
});
