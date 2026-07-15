const channels = ["stable", "dev"] as const;

export type DocsChannel = (typeof channels)[number];

const configuredChannel = process.env.CHATTO_DOCS_CHANNEL ?? "dev";

if (!channels.includes(configuredChannel as DocsChannel)) {
  throw new Error(
    `CHATTO_DOCS_CHANNEL must be one of ${channels.join(", ")}; received ${configuredChannel}`,
  );
}

export const docsChannel = configuredChannel as DocsChannel;
export const docsVersion =
  process.env.CHATTO_DOCS_VERSION ?? (docsChannel === "stable" ? undefined : "main");
export const docsRevision = process.env.CHATTO_DOCS_REVISION ?? "local";
export const docsSiteUrl =
  process.env.CHATTO_DOCS_SITE_URL ??
  (docsChannel === "stable"
    ? "https://docs.chatto.run"
    : "https://dev-docs.chatto.run");

if (!docsVersion) {
  throw new Error("CHATTO_DOCS_VERSION is required for stable documentation builds");
}

if (docsChannel === "stable" && !/^v\d+\.\d+\.\d+$/.test(docsVersion)) {
  throw new Error(
    `Stable CHATTO_DOCS_VERSION must be a stable Git tag such as v0.5.0; received ${docsVersion}`,
  );
}

export const docsShortRevision =
  docsRevision === "local" ? docsRevision : docsRevision.slice(0, 12);
export const docsDisplayVersion =
  docsChannel === "stable" ? docsVersion : `main@${docsShortRevision}`;
