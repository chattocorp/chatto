import { createLingua, type LocalizedHtml } from "../src/index.js";

const lingua = createLingua({
  baseLocale: "en-GB",
  loaders: {
    common: {
      "en-GB": async () => ({ common: { close: "Close" } }),
      "de-DE": async () => ({ common: { close: "Schließen" } }),
    },
  },
});

lingua.t("common.close");
lingua.t("common.welcome", { name: "Ada" });
lingua.t("common.member_count", { count: 3 });
lingua.t("common.member.count", { count: 3 });

// @ts-expect-error Count keys require parameters.
lingua.t("common.member_count");
// @ts-expect-error Count must be numeric.
lingua.t("common.member_count", { count: "three" });
// @ts-expect-error HTML keys must use html().
lingua.t("common.legal_html");
// @ts-expect-error html() only accepts HTML keys.
lingua.html("common.close");

const html: LocalizedHtml = lingua.html("common.legal_html");
const pluralHtml: LocalizedHtml = lingua.html("common.member_count_html", {
  count: 3,
});
void html;
void pluralHtml;

lingua.setLocale("de-DE");
lingua.setActiveSections(["common"]);

// @ts-expect-error Locale names come from the loader registry.
lingua.setLocale("fr");
// @ts-expect-error Section names come from the loader registry.
lingua.setActiveSections(["room"]);
