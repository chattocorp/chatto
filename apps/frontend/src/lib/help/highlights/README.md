# Release highlights

Write a highlight only when a user, administrator, or operator needs to know that
something works differently, a useful feature is available, or an action is
required. Do not add refactoring, dependency maintenance, test changes, or other
chores unless they have a concrete effect on these readers. Curate highlights
separately from the generated changelog. A release can have no highlights.

The `users` and `admins` collections define separate pages and dismissal state.
Give each entry a stable ID, its frontend release, a translated title key, and,
when necessary, the minimum server release for the described behavior. Keep the
ID when correcting or translating text. A new ID announces a new message.

Write the article in `content/<locale>/<surface>/<id>.mdx`. Keep it short and link
to Help for instructions. Review British English first, then translate each
complete locale. Regional inheritance follows the Help locale rules. Do not
publish incomplete translations. Keep article bodies out of the catalog so the
header does not load the articles or their import map.

MDsveX compiles this trusted application source during the frontend build. Never
fetch or execute MDX from a server. An older frontend can show plain-text feature
names and descriptions from `ServerDiscoveryService.ListFeatures` in the
**Available with a newer app** section. That catalog is advisory; permissions and
server configuration still control access. Unknown versions do not establish
feature support.

The first visit records the latest applicable release as the account's baseline
for that surface. Before that baseline exists, the badge uses the same release.
Later releases stay new until the reader explicitly dismisses their IDs. The
history remains readable. Dismissal and the baseline belong to the selected
server account, not a device or an identity shared across servers. Servers
without this API keep readable history but cannot synchronise dismissals.

Run the catalog and locale tests, frontend check, and production build. The build
compiles every MDX translation. Verify the Help badge, direct article links,
individual and bulk dismissal, account changes, and administration entry in a
browser at desktop and mobile sizes.
