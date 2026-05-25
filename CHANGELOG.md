# Changelog

All notable changes to Chatto. Maintained by release-please from the
conventional-commit messages on `main` — do not edit by hand.

## [0.0.188](https://github.com/chattocorp/chatto/compare/v0.0.187...v0.0.188) (2026-05-25)


### Features

* **docker:** ship nats CLI in production image, pre-wired to chatto's NATS ([#591](https://github.com/chattocorp/chatto/issues/591)) ([58ebfb1](https://github.com/chattocorp/chatto/commit/58ebfb1ddcc6690beb09b46aabdf4938c058e85d))


### Bug Fixes

* **assets:** per-user signed URLs so remote-server attachments load cross-origin ([#589](https://github.com/chattocorp/chatto/issues/589)) ([6f08d31](https://github.com/chattocorp/chatto/commit/6f08d31007d8b3ef357e89faa9e96cfd1d7420f8))
* **frontend:** refresh attachment URLs on lightbox open and download click ([#616](https://github.com/chattocorp/chatto/issues/616)) ([23973ac](https://github.com/chattocorp/chatto/commit/23973acb977e1cfa8b8149885c0ba23ce1e7a315))

## [0.0.187](https://github.com/chattocorp/chatto/compare/v0.0.186...v0.0.187) (2026-05-25)


### Features

* **docker:** ship nats CLI in production image, pre-wired to chatto's NATS ([#591](https://github.com/chattocorp/chatto/issues/591)) ([58ebfb1](https://github.com/chattocorp/chatto/commit/58ebfb1ddcc6690beb09b46aabdf4938c058e85d))
* **rooms:** seed announcements and general on fresh server boot ([#586](https://github.com/chattocorp/chatto/issues/586)) ([1a82f91](https://github.com/chattocorp/chatto/commit/1a82f918f6a096cc584ebf92ae918b82f34f0c9d))


### Bug Fixes

* **assets:** per-user signed URLs so remote-server attachments load cross-origin ([#589](https://github.com/chattocorp/chatto/issues/589)) ([6f08d31](https://github.com/chattocorp/chatto/commit/6f08d31007d8b3ef357e89faa9e96cfd1d7420f8))
* **assets:** probe storage backends when Attachment.Storage is missing ([#588](https://github.com/chattocorp/chatto/issues/588)) ([86f7b7c](https://github.com/chattocorp/chatto/commit/86f7b7c1abca4e57064ea63b9cf603b829ca3eb3))
* **frontend:** refresh attachment URLs on lightbox open and download click ([#616](https://github.com/chattocorp/chatto/issues/616)) ([23973ac](https://github.com/chattocorp/chatto/commit/23973acb977e1cfa8b8149885c0ba23ce1e7a315))

## [0.0.186](https://github.com/chattocorp/chatto/compare/v0.0.190...v0.0.186) (2026-05-25)


### ⚠ BREAKING CHANGES

* **proto:** drop space_id from live-only event protos ([#568](https://github.com/chattocorp/chatto/issues/568))
* **graphql:** joinRoom returns Room instead of Boolean ([#564](https://github.com/chattocorp/chatto/issues/564))
* **graphql:** rename Query.user(id:) argument to userId ([#563](https://github.com/chattocorp/chatto/issues/563))
* **graphql:** rename editMessage to updateMessage ([#562](https://github.com/chattocorp/chatto/issues/562))
* **graphql:** drop AdminQueries duplicates of Server role fields ([#561](https://github.com/chattocorp/chatto/issues/561))
* **graphql:** wrap deleteAvatar and clearUsernameCooldown in Input objects ([#560](https://github.com/chattocorp/chatto/issues/560))
* **graphql:** standardize permission list names on group inspectors ([#559](https://github.com/chattocorp/chatto/issues/559))
* **graphql:** rename thread-root references to threadRootEventId ([#558](https://github.com/chattocorp/chatto/issues/558))
* **graphql:** rename role to roleName in permission inputs ([#556](https://github.com/chattocorp/chatto/issues/556))
* **graphql:** remove duplicate AdminQueries.userRoles ([#557](https://github.com/chattocorp/chatto/issues/557))
* **graphql:** rename ConnectionInfo.serverID to serverId ([#555](https://github.com/chattocorp/chatto/issues/555))

### Features

* add Svelte MCP server configuration and documentation tools ([f2cccca](https://github.com/chattocorp/chatto/commit/f2cccca5f09f575e96f351e804bb329a71079563))
* **admin:** edit user identity and clear username cooldown ([#190](https://github.com/chattocorp/chatto/issues/190)) ([62f80b3](https://github.com/chattocorp/chatto/commit/62f80b3e1d1f632bb68bb357b94d3af53c2e4753))
* **api:** drop Space type from GraphQL surface; fold onto Instance ([#390](https://github.com/chattocorp/chatto/issues/390)) ([6308f21](https://github.com/chattocorp/chatto/commit/6308f21926d4bfa8187a859f8fb9702cea8dc863))
* **api:** drop spaceId from inputs/queries/events; rename SpaceEvent → RoomEvent ([#391](https://github.com/chattocorp/chatto/issues/391)) ([cfe6fc8](https://github.com/chattocorp/chatto/commit/cfe6fc8d7fcf526057a31a24743963fed5982657))
* **api:** expose messageEditWindowSeconds on Server ([#420](https://github.com/chattocorp/chatto/issues/420)) ([3dc85cc](https://github.com/chattocorp/chatto/commit/3dc85cc78359ed5f7c16fc428c2d66f2c216cb95))
* **api:** move instance logo/banner to InstanceConfig storage; drop description ([#393](https://github.com/chattocorp/chatto/issues/393)) ([8535ecd](https://github.com/chattocorp/chatto/commit/8535ecd4d965e17c8a30ecd729fb6ac4a193e603))
* **api:** rename Space*Event types to Server*Event; vocabulary cleanup ([#392](https://github.com/chattocorp/chatto/issues/392)) ([9e8e300](https://github.com/chattocorp/chatto/commit/9e8e300e74ce6149e823e2dcfc05a8e3a1916a59))
* auto-join primary space + drop createSpace ([#330](https://github.com/chattocorp/chatto/issues/330) phase 2 cont.) ([#333](https://github.com/chattocorp/chatto/issues/333)) ([775445b](https://github.com/chattocorp/chatto/commit/775445b5b16b79b8b259ba34ef529b78fb44a662))
* **bootstrap:** seed users + spaces from chatto.toml; replace setup wizard and admin.emails ([#252](https://github.com/chattocorp/chatto/issues/252)) ([c8ced4a](https://github.com/chattocorp/chatto/commit/c8ced4aa51605c41b98a8f838f3ca198dadaf676))
* **ci:** adopt release-please, retire `mise bump` ([#573](https://github.com/chattocorp/chatto/issues/573)) ([2eb2f67](https://github.com/chattocorp/chatto/commit/2eb2f678ac708316df7f04c3d8592308c7aa1c44))
* **cli:** add --include-keys flag to chatto backup ([#359](https://github.com/chattocorp/chatto/issues/359)) ([344bb5a](https://github.com/chattocorp/chatto/commit/344bb5a395f80c761268a9e66adb6ac33559081a))
* collapse [spaceId] out of chat URLs ([#330](https://github.com/chattocorp/chatto/issues/330) phase 2) ([#332](https://github.com/chattocorp/chatto/issues/332)) ([bdce0b2](https://github.com/chattocorp/chatto/commit/bdce0b2aaceb33f0132f20d97098ecda421e3b5e))
* **composer:** auto-focus on room/DM navigation, skip on touch ([#340](https://github.com/chattocorp/chatto/issues/340)) ([a5b8401](https://github.com/chattocorp/chatto/commit/a5b84012eff511050fdb516cb7344d5feb52bff4))
* consolidate server branding & fold instance-admin into server-admin ([#395](https://github.com/chattocorp/chatto/issues/395)) ([cee9eb6](https://github.com/chattocorp/chatto/commit/cee9eb6b9ce8605db7e21971312cf0b3b1cd35fa))
* **core:** reserve "here" and "all" as group names ([#462](https://github.com/chattocorp/chatto/issues/462)) ([e925377](https://github.com/chattocorp/chatto/commit/e92537784b49fac15d1a5295fc6da8b91508ee21))
* **dev:** restore frontend hot reload via separate Vite service ([#485](https://github.com/chattocorp/chatto/issues/485)) ([836dc7b](https://github.com/chattocorp/chatto/commit/836dc7b5ecd45516aea84d4947848ba46b7bb158))
* **docker:** ship nats CLI in production image, pre-wired to chatto's NATS ([#591](https://github.com/chattocorp/chatto/issues/591)) ([58ebfb1](https://github.com/chattocorp/chatto/commit/58ebfb1ddcc6690beb09b46aabdf4938c058e85d))
* drop /instances page, fold disconnect into Leave Server ([#330](https://github.com/chattocorp/chatto/issues/330)) ([#349](https://github.com/chattocorp/chatto/issues/349)) ([636ba23](https://github.com/chattocorp/chatto/commit/636ba23c9e582333f91506664b917e9f6b0be367))
* expose inThread on MentionNotificationItem (and remove /m/ workaround) ([#207](https://github.com/chattocorp/chatto/issues/207)) ([4fd4f19](https://github.com/chattocorp/chatto/commit/4fd4f19c702dafd16e764e406f6ff52f9652c696))
* **frontend:** add hover highlight to app header icons ([#425](https://github.com/chattocorp/chatto/issues/425)) ([0d99e87](https://github.com/chattocorp/chatto/commit/0d99e87da14e075685587556e0358ff337040b99))
* **frontend:** add per-server "Recently Used" section to emoji picker ([#428](https://github.com/chattocorp/chatto/issues/428)) ([2558e93](https://github.com/chattocorp/chatto/commit/2558e938e83a0a489784b85ff6a336e03e7032a5))
* **frontend:** add server overview entries to Cmd-K quick switcher ([#506](https://github.com/chattocorp/chatto/issues/506)) ([5132875](https://github.com/chattocorp/chatto/commit/5132875d71b0b8a3b15206145122305e88ea1125))
* **frontend:** align dialogs with menu language; FormDialog, tones, Add Instance modal ([#261](https://github.com/chattocorp/chatto/issues/261)) ([56e6056](https://github.com/chattocorp/chatto/commit/56e6056fdc973c0a68375b83c76aa10a1c413a80))
* **frontend:** anchor the unread separator when the tab loses focus ([#466](https://github.com/chattocorp/chatto/issues/466)) ([47d8f09](https://github.com/chattocorp/chatto/commit/47d8f0979df26ae9bbce9802bbf514c479e0e8e9))
* **frontend:** auto-reload client on new version when user is idle ([#570](https://github.com/chattocorp/chatto/issues/570)) ([391fe3f](https://github.com/chattocorp/chatto/commit/391fe3ff0eb7bee3f7da0dcc1925ac082b26c191))
* **frontend:** bring up Storybook with design system inventory ([#248](https://github.com/chattocorp/chatto/issues/248)) ([f0376db](https://github.com/chattocorp/chatto/commit/f0376db81b739716a6756611d0862997663c40f7))
* **frontend:** InstancePill — subtle cross-instance label with hover card ([#208](https://github.com/chattocorp/chatto/issues/208)) ([67af4f7](https://github.com/chattocorp/chatto/commit/67af4f7e1d321ed43e9eb2308eb5f30324fcfc0e))
* **frontend:** link joined rooms on the Overview page ([#496](https://github.com/chattocorp/chatto/issues/496)) ([a8fc49e](https://github.com/chattocorp/chatto/commit/a8fc49eb3d867da9076505fa5498dc08dbb7ff8e))
* **frontend:** make member groups collapsible ([#419](https://github.com/chattocorp/chatto/issues/419)) ([d6f6fe4](https://github.com/chattocorp/chatto/commit/d6f6fe493741961b51fd2828a114978a951e99ce))
* **frontend:** move current user info into secondary sidebar ([#507](https://github.com/chattocorp/chatto/issues/507)) ([de6b298](https://github.com/chattocorp/chatto/commit/de6b2985bdec5fea22cc59a85bbba68d19520427))
* **frontend:** open InstancePill card on click, add shimmer hover ([#257](https://github.com/chattocorp/chatto/issues/257)) ([4c8a53d](https://github.com/chattocorp/chatto/commit/4c8a53dacabb4c95168e65b5e434ede8247d7da7))
* **frontend:** pin first 4 quick-reaction slots, recents fill the last 2 ([#263](https://github.com/chattocorp/chatto/issues/263)) ([9600576](https://github.com/chattocorp/chatto/commit/9600576ab72648e4030e9e6f8898e068f0be7f17))
* **frontend:** redesign user card and add scroll fade overlays ([#524](https://github.com/chattocorp/chatto/issues/524)) ([2ed8766](https://github.com/chattocorp/chatto/commit/2ed876602c4da0a6e7d59a0f0d1aac41b2364f8c))
* **frontend:** resizable secondary sidebar and members pane ([#403](https://github.com/chattocorp/chatto/issues/403)) ([8bb28ad](https://github.com/chattocorp/chatto/commit/8bb28ad19768c05a7fe35594f4d2835d6a7c0eee))
* **frontend:** restructure reply preview and edited marker on messages ([#212](https://github.com/chattocorp/chatto/issues/212)) ([5e930d2](https://github.com/chattocorp/chatto/commit/5e930d2561b3a482362168a4a078e732b641bf0b))
* instance-wide limits for spaces and verified users ([#214](https://github.com/chattocorp/chatto/issues/214)) ([093fe4a](https://github.com/chattocorp/chatto/commit/093fe4ae74f3cc553918e02b393e9bcba16763ae))
* message links with previews and jump-to-message navigation ([#162](https://github.com/chattocorp/chatto/issues/162)) ([d82564f](https://github.com/chattocorp/chatto/commit/d82564f02e0e698b6e58ad767660f230a37a8814))
* **messages:** always render deleted messages as a tombstone ([#449](https://github.com/chattocorp/chatto/issues/449)) ([5681ba7](https://github.com/chattocorp/chatto/commit/5681ba79bda7d92bd301442a186e3791e7b13094))
* **migration:** phase 4a — primary's CONFIG/RBAC/RUNTIME → SERVER_* ([#354](https://github.com/chattocorp/chatto/issues/354)) ([#372](https://github.com/chattocorp/chatto/issues/372)) ([b07124c](https://github.com/chattocorp/chatto/commit/b07124c68c1482f4079b39d70045cf878c73d1ad))
* **migration:** phase 4b — DM merge, kind in key prefix ([#354](https://github.com/chattocorp/chatto/issues/354)) ([#373](https://github.com/chattocorp/chatto/issues/373)) ([655f331](https://github.com/chattocorp/chatto/commit/655f331f059585568a66cf2fd6292975a30a5c6c))
* **migration:** phase 4c — per-message KVs → SERVER_* ([#354](https://github.com/chattocorp/chatto/issues/354)) ([#376](https://github.com/chattocorp/chatto/issues/376)) ([4b3f2cb](https://github.com/chattocorp/chatto/commit/4b3f2cb5f02860d469e64be40773774c2720fd8f))
* **migration:** phase 4d — per-space event streams → SERVER_EVENTS ([#354](https://github.com/chattocorp/chatto/issues/354)) ([#381](https://github.com/chattocorp/chatto/issues/381)) ([022054d](https://github.com/chattocorp/chatto/commit/022054d6924da58de8040636a0400045a1526b66))
* **migration:** phase 4e — per-space attachments → SERVER_ASSETS ([#354](https://github.com/chattocorp/chatto/issues/354)) ([#378](https://github.com/chattocorp/chatto/issues/378)) ([e8a8a3b](https://github.com/chattocorp/chatto/commit/e8a8a3bf32fb61a57c8f938d9e9f618f891110b3))
* **migration:** phase 4f — fresh-install bootstrap polish ([#354](https://github.com/chattocorp/chatto/issues/354)) ([#383](https://github.com/chattocorp/chatto/issues/383)) ([6dd3555](https://github.com/chattocorp/chatto/commit/6dd35553d0b9294ba4f11fd1d3ddd049d1d7c585))
* **migration:** phase 4g — bridge retirement ([#354](https://github.com/chattocorp/chatto/issues/354)) ([#384](https://github.com/chattocorp/chatto/issues/384)) ([071e7c4](https://github.com/chattocorp/chatto/commit/071e7c41096f9223b63726cafdb9d42b4149417d))
* **mobile:** drag-to-dismiss bottom sheets, panGesture primitive ([#355](https://github.com/chattocorp/chatto/issues/355)) ([9aec0c5](https://github.com/chattocorp/chatto/commit/9aec0c5de4c09fa12e68132553592ffbb4e6a764))
* **mobile:** slide-in sidebar with edge-swipe gesture ([#353](https://github.com/chattocorp/chatto/issues/353)) ([684be83](https://github.com/chattocorp/chatto/commit/684be83522ef75dc691668dd71723560c339cac2))
* **quick-switcher:** add header button to open palette ([#351](https://github.com/chattocorp/chatto/issues/351)) ([f7aeb76](https://github.com/chattocorp/chatto/commit/f7aeb766d21da6a979f3f5eaf6b4b75857afe104))
* **quick-switcher:** match against server name and support multi-token queries ([#345](https://github.com/chattocorp/chatto/issues/345)) ([170c14d](https://github.com/chattocorp/chatto/commit/170c14d01d0e035cca6bb01869920e6bca14ba68))
* **rbac:** collapse dual-tier RBAC into unified SERVER_RBAC ([#357](https://github.com/chattocorp/chatto/issues/357)) ([#398](https://github.com/chattocorp/chatto/issues/398)) ([33f2220](https://github.com/chattocorp/chatto/commit/33f2220d52eb4ad2d51bde2da8147ff254c68663))
* **rbac:** permission inspector + unified role-management UI ([#193](https://github.com/chattocorp/chatto/issues/193)) ([72b73c4](https://github.com/chattocorp/chatto/commit/72b73c47aaec19e7ab52a9a4cff0e4c33c9e4ced))
* **rbac:** unified permission matrix UI ([#195](https://github.com/chattocorp/chatto/issues/195)) ([9a50dfe](https://github.com/chattocorp/chatto/commit/9a50dfebbb3dfe7aaf63189b063d9f6d4208275a))
* rename Add Instance → Add Server, inline preview before OAuth ([#330](https://github.com/chattocorp/chatto/issues/330)) ([#341](https://github.com/chattocorp/chatto/issues/341)) ([116b526](https://github.com/chattocorp/chatto/commit/116b52652aa557e072b5f895b07c9e2578c8d6e6))
* room-group-centric ACL (ADR-031) + admin UI rework ([#464](https://github.com/chattocorp/chatto/issues/464)) ([547bdc6](https://github.com/chattocorp/chatto/commit/547bdc63ad21b2910f3b4518dbc9075c21256d8f))
* **rooms:** group consecutive join/leave events in the timeline ([#440](https://github.com/chattocorp/chatto/issues/440)) ([97e3aa0](https://github.com/chattocorp/chatto/commit/97e3aa09be6006625745e8c1117a54e2a90a6987))
* **rooms:** seed announcements and general on fresh server boot ([#586](https://github.com/chattocorp/chatto/issues/586)) ([1a82f91](https://github.com/chattocorp/chatto/commit/1a82f918f6a096cc584ebf92ae918b82f34f0c9d))
* **server:** primary-space config bridge + ADR-027 ([#330](https://github.com/chattocorp/chatto/issues/330) phase 1) ([#331](https://github.com/chattocorp/chatto/issues/331)) ([540be0f](https://github.com/chattocorp/chatto/commit/540be0fbb81828f9e60b009c67f739c512e9c70c))
* **sidebar:** wrap ungrouped rooms in a collapsible group, polish carets ([#337](https://github.com/chattocorp/chatto/issues/337)) ([b7af817](https://github.com/chattocorp/chatto/commit/b7af8178a3de2dadf42eb318624f313e5f218801))
* **subjects:** phase 4d prep — primary-aware subject dispatch ([#354](https://github.com/chattocorp/chatto/issues/354)) ([#379](https://github.com/chattocorp/chatto/issues/379)) ([dfc78f3](https://github.com/chattocorp/chatto/commit/dfc78f3ddb8fa93ffc78c6ef9f3b628250dab104))
* **subscriptions:** detect and recover from dead GraphQL subscriptions ([#423](https://github.com/chattocorp/chatto/issues/423)) ([a3beac0](https://github.com/chattocorp/chatto/commit/a3beac0a5abddef41e9eda4d9e6dd16bdef51818))
* support Tilt-based Kubernetes dev environment ([7e00c5b](https://github.com/chattocorp/chatto/commit/7e00c5b51b1e8df091d94e111cc2a1a07155e950))
* surface DMs under primary Space.rooms ([#330](https://github.com/chattocorp/chatto/issues/330) phase 3) ([#335](https://github.com/chattocorp/chatto/issues/335)) ([d49d7d9](https://github.com/chattocorp/chatto/commit/d49d7d9a881bb6465457a425d33a5ecf61a09f08))
* **tools:** GraphQL authorization matrix fuzzer ([#181](https://github.com/chattocorp/chatto/issues/181)) ([5ce01cd](https://github.com/chattocorp/chatto/commit/5ce01cdbadb68ac804f2b687997c0f0e5a82e7da))


### Bug Fixes

* add .claude/scheduled_tasks.lock to .gitignore ([a7ab229](https://github.com/chattocorp/chatto/commit/a7ab229d6831ec58851dcf30aedffb3ab4236677))
* add .claude/worktrees/ to .gitignore ([ad2a2e8](https://github.com/chattocorp/chatto/commit/ad2a2e8a63cce623500682d7ad19021df5bab15f))
* **admin:** exclude DM rooms from server-admin Manage rooms page ([#386](https://github.com/chattocorp/chatto/issues/386)) ([9b27032](https://github.com/chattocorp/chatto/commit/9b270328f514a00bb5e8a9f88c090f6d6b81d275))
* **admin:** resolve viewer permissions against the active instance ([#415](https://github.com/chattocorp/chatto/issues/415)) ([d497590](https://github.com/chattocorp/chatto/commit/d497590e9b0dc2bb049d8406cf878d6001176070))
* **assets:** authorize attachment downloads via canonical Attachment records ([#575](https://github.com/chattocorp/chatto/issues/575)) ([c3ab155](https://github.com/chattocorp/chatto/commit/c3ab155deb72c3c1781457c3773bab7402c2519c))
* **assets:** per-user signed URLs so remote-server attachments load cross-origin ([#589](https://github.com/chattocorp/chatto/issues/589)) ([6f08d31](https://github.com/chattocorp/chatto/commit/6f08d31007d8b3ef357e89faa9e96cfd1d7420f8))
* **assets:** probe storage backends when Attachment.Storage is missing ([#588](https://github.com/chattocorp/chatto/issues/588)) ([86f7b7c](https://github.com/chattocorp/chatto/commit/86f7b7c1abca4e57064ea63b9cf603b829ca3eb3))
* **auth:** cap password length at 128 bytes ([#405](https://github.com/chattocorp/chatto/issues/405)) ([c13d953](https://github.com/chattocorp/chatto/commit/c13d95360fe42783251553a87cbb83c82ae04436))
* **bootstrap:** auto-join bootstrap users to the primary space ([#336](https://github.com/chattocorp/chatto/issues/336)) ([95c4a85](https://github.com/chattocorp/chatto/commit/95c4a85b5109a8df67602317b1d8f3457387df4b))
* **ci:** point goreleaser at the chattocorp org ([c5bfc6c](https://github.com/chattocorp/chatto/commit/c5bfc6c7bf1e24b55ff649f10c69cba53bc386f9))
* **ci:** use built-in GITHUB_TOKEN for goreleaser ([a9c304f](https://github.com/chattocorp/chatto/commit/a9c304fea218b93a3e730cf3c9be0bbd7f0acea9))
* **ci:** use PAT for goreleaser so release notes autolink ([bc13406](https://github.com/chattocorp/chatto/commit/bc1340623f920013bc470c93ab4ceaa200a67ac2))
* **composer:** keep mobile keyboard stable across message send ([12a004e](https://github.com/chattocorp/chatto/commit/12a004e7ac21161e6f952f6ec0ea022b7ac771c0))
* **core:** derive inThread from inReplyTo target when caller omits it ([#250](https://github.com/chattocorp/chatto/issues/250)) ([a63764a](https://github.com/chattocorp/chatto/commit/a63764a444deca49c963c3a447667ddf40bae453))
* **core:** use FlushTimeout for NATS event publishes ([#245](https://github.com/chattocorp/chatto/issues/245)) ([9751a5b](https://github.com/chattocorp/chatto/commit/9751a5b66cb24b6b876e30a037cec044f00e22c6))
* **dev:** colocate .svelte-kit with node_modules in compose stack ([#467](https://github.com/chattocorp/chatto/issues/467)) ([0c8c7f3](https://github.com/chattocorp/chatto/commit/0c8c7f3c06c88a5ec2b64dd8a50c2a3ae4d1d643))
* **dev:** poll for file changes in dev containers ([#191](https://github.com/chattocorp/chatto/issues/191)) ([a82da92](https://github.com/chattocorp/chatto/commit/a82da9209a51176464d2551f3cf1e1e592df4c65))
* **e2e:** eliminate flaky tier by fixing thread-pane and pagination races ([#163](https://github.com/chattocorp/chatto/issues/163)) ([cf6245b](https://github.com/chattocorp/chatto/commit/cf6245b29c579442cae54f70cfcfa283f3087db3))
* **e2e:** eliminate two deterministic flakes in the e2e suite ([#375](https://github.com/chattocorp/chatto/issues/375)) ([9988388](https://github.com/chattocorp/chatto/commit/9988388186c1070303820c3f72a91eeb4967d54b))
* **e2e:** w=2 workers + idempotent logout-dialog click retry ([#377](https://github.com/chattocorp/chatto/issues/377)) ([101112c](https://github.com/chattocorp/chatto/commit/101112cf903690abf3d4504eac7d730256a80878))
* **frontend:** align avatars, icons, and text across rows ([#501](https://github.com/chattocorp/chatto/issues/501)) ([5a9a742](https://github.com/chattocorp/chatto/commit/5a9a7422f10f47b8b5d546e9a0d040a456a81c08))
* **frontend:** avoid full reload when clicking push notifications ([#487](https://github.com/chattocorp/chatto/issues/487)) ([08c547a](https://github.com/chattocorp/chatto/commit/08c547a0149c85520641813dab741b2bc8beb108))
* **frontend:** break redirect loop when lastRoom/lastSpace is stale ([#167](https://github.com/chattocorp/chatto/issues/167)) ([72498b3](https://github.com/chattocorp/chatto/commit/72498b3de7415e0a466e396cc12c34b93f846d43))
* **frontend:** deleted messages stay on screen for other users until refresh ([#194](https://github.com/chattocorp/chatto/issues/194)) ([de74f6b](https://github.com/chattocorp/chatto/commit/de74f6b445382bcaebe9b98e39473fda3419a731))
* **frontend:** exclude DMs from Preferences &gt; Room Overrides list ([#396](https://github.com/chattocorp/chatto/issues/396)) ([cea2de9](https://github.com/chattocorp/chatto/commit/cea2de938fd5abeae24596ded4e957f67692e352))
* **frontend:** gate Jump to Present on user-driven scroll signals ([#447](https://github.com/chattocorp/chatto/issues/447)) ([9106f7b](https://github.com/chattocorp/chatto/commit/9106f7b81cbe688cf29ebcd5efb1b1184cc1dbc7))
* **frontend:** isolate per-instance failures in multi-instance client ([#370](https://github.com/chattocorp/chatto/issues/370)) ([b2a32ee](https://github.com/chattocorp/chatto/commit/b2a32ee11a174f5f2aa673bbdcc3abca6e37f34f))
* **frontend:** keep Add Server dialog open on mobile ([#532](https://github.com/chattocorp/chatto/issues/532)) ([616c974](https://github.com/chattocorp/chatto/commit/616c97446a43d1f4e59a9d1e3b0ad0c360c11430))
* **frontend:** keep bottom sheet open on Android keyboard cancel event ([#531](https://github.com/chattocorp/chatto/issues/531)) ([c34131e](https://github.com/chattocorp/chatto/commit/c34131e4716b277769c51a4444e41c2986b555a6))
* **frontend:** keep dialog open when drag starts inside and ends outside ([#469](https://github.com/chattocorp/chatto/issues/469)) ([8224d74](https://github.com/chattocorp/chatto/commit/8224d74d0b866f9595d64596a214dc536648e4dd))
* **frontend:** keep mobile bottom sheet open when tapping inputs ([#525](https://github.com/chattocorp/chatto/issues/525)) ([c27d588](https://github.com/chattocorp/chatto/commit/c27d58873ec3acafc05fc7872db2385f5b2c97a3))
* **frontend:** keep top margin on leading blockquote in messages ([#486](https://github.com/chattocorp/chatto/issues/486)) ([444bc8e](https://github.com/chattocorp/chatto/commit/444bc8e4c7ef90fe32d4a82a517cab3f6aa6d5a5))
* **frontend:** keep unread separator stable across tab refocus ([#513](https://github.com/chattocorp/chatto/issues/513)) ([2139887](https://github.com/chattocorp/chatto/commit/21398874f7675e35f98c6f3543fac2e536e9131b))
* **frontend:** keep WS retry loop alive so tab-resume reconnects work ([#502](https://github.com/chattocorp/chatto/issues/502)) ([10e2a0d](https://github.com/chattocorp/chatto/commit/10e2a0d8574d71b7fcde687fb426ab9ff8481d63))
* **frontend:** land AuthenticatedChatProvider on registry's CurrentUserState ([#184](https://github.com/chattocorp/chatto/issues/184)) ([cb362f1](https://github.com/chattocorp/chatto/commit/cb362f1453a462e6337f8f793a0ed7dea3d0a6b3))
* **frontend:** land scroll at true bottom on room entry ([#530](https://github.com/chattocorp/chatto/issues/530)) ([f3a7e80](https://github.com/chattocorp/chatto/commit/f3a7e8045a711e98280edea486b06320a6dee529))
* **frontend:** lower threshold for showing Jump to Present button ([#242](https://github.com/chattocorp/chatto/issues/242)) ([2f44883](https://github.com/chattocorp/chatto/commit/2f44883e0959f3eae22834386400224ebe9a2290))
* **frontend:** MOTD reflects the currently-viewed server ([#407](https://github.com/chattocorp/chatto/issues/407)) ([1e1ae8c](https://github.com/chattocorp/chatto/commit/1e1ae8cb6bd8a73ce22875b2d47a5988b50ea5ca))
* **frontend:** orange-dot click no longer scrolls to bottom of wrong room ([#196](https://github.com/chattocorp/chatto/issues/196)) ([e2d0d2d](https://github.com/chattocorp/chatto/commit/e2d0d2dfd39e71fa9e584a87a6582307c7fcea57))
* **frontend:** per-server stores stay in sync when switching servers ([#444](https://github.com/chattocorp/chatto/issues/444)) ([6679ece](https://github.com/chattocorp/chatto/commit/6679ece99df1f8125766c20b78adb7b3d42acd8a))
* **frontend:** polish Direct Messages list rendering ([#205](https://github.com/chattocorp/chatto/issues/205)) ([76ba53e](https://github.com/chattocorp/chatto/commit/76ba53efdfe7ee1c4be603698f541b9522b489c8))
* **frontend:** preserve last-space/last-room on transient reconnect failures ([#164](https://github.com/chattocorp/chatto/issues/164)) ([98da25f](https://github.com/chattocorp/chatto/commit/98da25fc638771f2340a65449773c027abc19730))
* **frontend:** preserve literal backslashes in rendered messages ([#173](https://github.com/chattocorp/chatto/issues/173)) ([1e3e4ce](https://github.com/chattocorp/chatto/commit/1e3e4cec1ce5f3180cce82405f4e6ec21e0f3710))
* **frontend:** prevent embed/attachment overflow on mobile viewports ([#410](https://github.com/chattocorp/chatto/issues/410)) ([aab1b76](https://github.com/chattocorp/chatto/commit/aab1b7656f25da0896e18a70d14f5a6e56a17236))
* **frontend:** prevent SpaceBanner from letterboxing at narrow widths ([#427](https://github.com/chattocorp/chatto/issues/427)) ([75617bb](https://github.com/chattocorp/chatto/commit/75617bb93a2908892100361752002d5479d93ff1))
* **frontend:** re-check Jump to Present visibility on tab resume ([#452](https://github.com/chattocorp/chatto/issues/452)) ([5f3999c](https://github.com/chattocorp/chatto/commit/5f3999c5ea7c544e95b02e2ad2dd6b6829681248))
* **frontend:** refresh attachment URLs on lightbox open and download click ([#616](https://github.com/chattocorp/chatto/issues/616)) ([23973ac](https://github.com/chattocorp/chatto/commit/23973acb977e1cfa8b8149885c0ba23ce1e7a315))
* **frontend:** render `_...word_` as italic when preceded by punctuation ([#197](https://github.com/chattocorp/chatto/issues/197)) ([9e65057](https://github.com/chattocorp/chatto/commit/9e65057102caf69ca2456737479b753217761e93))
* **frontend:** render `**foo:**` as bold; add ATX heading support ([#249](https://github.com/chattocorp/chatto/issues/249)) ([e4a5536](https://github.com/chattocorp/chatto/commit/e4a55362eff7a55b2b29fc39a4ad24cda9a4c287))
* **frontend:** reset Create Room form on each open ([#246](https://github.com/chattocorp/chatto/issues/246)) ([1d844e8](https://github.com/chattocorp/chatto/commit/1d844e88b7fb429bd152768c4a0ec82fcfac488a))
* **frontend:** restore DM list ordering, unread dot, notification dot ([#264](https://github.com/chattocorp/chatto/issues/264)) ([d1d9831](https://github.com/chattocorp/chatto/commit/d1d9831cee4f8ae002e93743e5e5b1315ad7471a))
* **frontend:** restore per-server last-room memory on server re-entry ([#511](https://github.com/chattocorp/chatto/issues/511)) ([9cc60c1](https://github.com/chattocorp/chatto/commit/9cc60c1e3d2495e7ba45621213acd20edd030ef2))
* **frontend:** restore vertical separator between sidebars on mobile ([#515](https://github.com/chattocorp/chatto/issues/515)) ([144d1c0](https://github.com/chattocorp/chatto/commit/144d1c0a8664f2c643ef281a2d65ccc21b62619e))
* **frontend:** scale space banner correctly in Safari ([#421](https://github.com/chattocorp/chatto/issues/421)) ([43a52e1](https://github.com/chattocorp/chatto/commit/43a52e15c7271781af50ecc6170c8e9d4b71e54f))
* **frontend:** scroll autocomplete popup selection into view ([#465](https://github.com/chattocorp/chatto/issues/465)) ([d3af465](https://github.com/chattocorp/chatto/commit/d3af46546e3efeb2907f546fef25f6bce369adef))
* **frontend:** scroll thread to new reply on notification click ([#192](https://github.com/chattocorp/chatto/issues/192)) ([683df99](https://github.com/chattocorp/chatto/commit/683df992354237e7604dc935dab01634ff828710))
* **frontend:** show skeleton immediately on room switch ([#213](https://github.com/chattocorp/chatto/issues/213)) ([fc720bc](https://github.com/chattocorp/chatto/commit/fc720bc6ea1a4d6c090896e4b6a3f75fa2c4b4e9))
* **frontend:** stop blank lines doubling when editing messages ([#170](https://github.com/chattocorp/chatto/issues/170)) ([e2a3a42](https://github.com/chattocorp/chatto/commit/e2a3a428bccbcd6b2cba6e821d679f1d746b6bc7))
* **frontend:** use base font size for username change notice ([#508](https://github.com/chattocorp/chatto/issues/508)) ([7af5e1a](https://github.com/chattocorp/chatto/commit/7af5e1af4d919036180cd8554a2cfac2ca754b7e))
* **link-preview:** suppress card when target page has no OG metadata ([#343](https://github.com/chattocorp/chatto/issues/343)) ([8421949](https://github.com/chattocorp/chatto/commit/8421949978c449da7a9a3d1f87eb46f9c41a8b30))
* **migrations:** backfill records for video variants and thumbnails ([#577](https://github.com/chattocorp/chatto/issues/577)) ([ca43ce8](https://github.com/chattocorp/chatto/commit/ca43ce8300101ea679dfc7066c2b588db7a815c0))
* **push:** align notification URLs with collapsed /chat/[instanceId]/[roomId] shape ([#338](https://github.com/chattocorp/chatto/issues/338)) ([8bcf9e9](https://github.com/chattocorp/chatto/commit/8bcf9e973b3d747ed803a6ef083e74cb32804f9a))
* **quick-switcher:** only match existing DMs, not arbitrary users ([#342](https://github.com/chattocorp/chatto/issues/342)) ([38bfce7](https://github.com/chattocorp/chatto/commit/38bfce7091fad1675515707182e6068b10201aca))
* **rbac:** rename PermissionLevel to SERVER/GROUP/ROOM, fix group mislabel ([#503](https://github.com/chattocorp/chatto/issues/503)) ([1dc2c0d](https://github.com/chattocorp/chatto/commit/1dc2c0d88f1c361dffe51d624d489409a0703e88))
* require display name to start with letter or digit ([#237](https://github.com/chattocorp/chatto/issues/237)) ([b3833f7](https://github.com/chattocorp/chatto/commit/b3833f79b56d4b72c9374d24dac4b94ec1d4514e))
* **restore:** report whether encryption keys were actually restored ([#361](https://github.com/chattocorp/chatto/issues/361)) ([65478ba](https://github.com/chattocorp/chatto/commit/65478ba5ef587ca2fa9059f7f221545fd30b8e93))
* **rooms:** drive read-marker off user presence (focus + visibility) ([#448](https://github.com/chattocorp/chatto/issues/448)) ([18bc71f](https://github.com/chattocorp/chatto/commit/18bc71fa3ba72a13e7900a56693f2eb33caf2f98))
* **rooms:** key system-groups by newest event to stabilize scroll-up ([#445](https://github.com/chattocorp/chatto/issues/445)) ([3d7e6b1](https://github.com/chattocorp/chatto/commit/3d7e6b1fe077f1de050917d0f07c36b1d4514f5e))
* **sidebar:** exclude self from DM label and avatars reliably ([#339](https://github.com/chattocorp/chatto/issues/339)) ([4b9d588](https://github.com/chattocorp/chatto/commit/4b9d588372fa26a96518a49f587de933641b2de6))
* **thread:** scroll thread pane to bottom on own reply ([#385](https://github.com/chattocorp/chatto/issues/385)) ([5b8bfde](https://github.com/chattocorp/chatto/commit/5b8bfdec3c262e98665e5fc43c7c96d3d2e64d43))
* **ui:** cleaner space-banner bleed using mirrored copies ([#438](https://github.com/chattocorp/chatto/issues/438)) ([c8b664a](https://github.com/chattocorp/chatto/commit/c8b664a598984437ce01b29091e63a5fcafa2be9))
* **ui:** use mdi--login icon for sign-in buttons ([#450](https://github.com/chattocorp/chatto/issues/450)) ([259a633](https://github.com/chattocorp/chatto/commit/259a633e5819d16b80e57dfd8ed12f56f5ae29cc))
* update watchexec version in mise.toml ([c9d3a57](https://github.com/chattocorp/chatto/commit/c9d3a57d2f1369df355faa6e57bf586026cda271))
* use editor content for up-arrow edit guard ([#617](https://github.com/chattocorp/chatto/issues/617)) ([7d79d9f](https://github.com/chattocorp/chatto/commit/7d79d9f22582afe1f7c2b79da1c06f99eabc2712))


### Reverts

* re-enable GraphQL introspection and /api/playground for everyone ([#206](https://github.com/chattocorp/chatto/issues/206)) ([0a1b641](https://github.com/chattocorp/chatto/commit/0a1b641d107ab19b04f72adb6226100e33b27103))


### Miscellaneous Chores

* cut release 0.0.186 ([3f6e05e](https://github.com/chattocorp/chatto/commit/3f6e05e9899bb3dff94e7a2bf16f662b59e57b6c))


### Code Refactoring

* **graphql:** drop AdminQueries duplicates of Server role fields ([#561](https://github.com/chattocorp/chatto/issues/561)) ([ae4f6c4](https://github.com/chattocorp/chatto/commit/ae4f6c4c7f223ddb1115ee3a0004b7731fb2f692))
* **graphql:** joinRoom returns Room instead of Boolean ([#564](https://github.com/chattocorp/chatto/issues/564)) ([18fb138](https://github.com/chattocorp/chatto/commit/18fb1389be41220bb42bd0780e808fa105555c92))
* **graphql:** remove duplicate AdminQueries.userRoles ([#557](https://github.com/chattocorp/chatto/issues/557)) ([0c1489f](https://github.com/chattocorp/chatto/commit/0c1489f4eeef9ac163f0442bdf8f5bb2ffee05b5))
* **graphql:** rename ConnectionInfo.serverID to serverId ([#555](https://github.com/chattocorp/chatto/issues/555)) ([e031620](https://github.com/chattocorp/chatto/commit/e031620c7e977c9aef5864d3532d4c5ab5540d32))
* **graphql:** rename editMessage to updateMessage ([#562](https://github.com/chattocorp/chatto/issues/562)) ([50a8ade](https://github.com/chattocorp/chatto/commit/50a8ade4ca1ec0cadb1baea47611f375edbf60df))
* **graphql:** rename Query.user(id:) argument to userId ([#563](https://github.com/chattocorp/chatto/issues/563)) ([db159b7](https://github.com/chattocorp/chatto/commit/db159b765f60c481006dbb550e5606333b4ba17e))
* **graphql:** rename role to roleName in permission inputs ([#556](https://github.com/chattocorp/chatto/issues/556)) ([cd77568](https://github.com/chattocorp/chatto/commit/cd77568ec0362a3e17b69e2606048cf7ee0c7d06))
* **graphql:** rename thread-root references to threadRootEventId ([#558](https://github.com/chattocorp/chatto/issues/558)) ([709a56c](https://github.com/chattocorp/chatto/commit/709a56c779e6fe73eef6acaa9a5e6ed5192eda57))
* **graphql:** standardize permission list names on group inspectors ([#559](https://github.com/chattocorp/chatto/issues/559)) ([cd0378a](https://github.com/chattocorp/chatto/commit/cd0378ae7be43d646e1b33ca00888a61f1578a43))
* **graphql:** wrap deleteAvatar and clearUsernameCooldown in Input objects ([#560](https://github.com/chattocorp/chatto/issues/560)) ([ee3ba83](https://github.com/chattocorp/chatto/commit/ee3ba83ce233140b23b1409ccee1167d0d548c10))
* **proto:** drop space_id from live-only event protos ([#568](https://github.com/chattocorp/chatto/issues/568)) ([b998f2f](https://github.com/chattocorp/chatto/commit/b998f2f515e44c90606797d4e24f59b1e19d043c))

## [0.0.190](https://github.com/chattocorp/chatto/compare/v0.0.189...v0.0.190) (2026-05-25)


### Bug Fixes

* **frontend:** refresh attachment URLs on lightbox open and download click ([#616](https://github.com/chattocorp/chatto/issues/616)) ([23973ac](https://github.com/chattocorp/chatto/commit/23973acb977e1cfa8b8149885c0ba23ce1e7a315))

## [0.0.189](https://github.com/chattocorp/chatto/compare/v0.0.188...v0.0.189) (2026-05-24)


### Features

* **docker:** ship nats CLI in production image, pre-wired to chatto's NATS ([#591](https://github.com/chattocorp/chatto/issues/591)) ([58ebfb1](https://github.com/chattocorp/chatto/commit/58ebfb1ddcc6690beb09b46aabdf4938c058e85d))

## [0.0.188](https://github.com/chattocorp/chatto/compare/v0.0.187...v0.0.188) (2026-05-24)


### Bug Fixes

* **assets:** per-user signed URLs so remote-server attachments load cross-origin ([#589](https://github.com/chattocorp/chatto/issues/589)) ([6f08d31](https://github.com/chattocorp/chatto/commit/6f08d31007d8b3ef357e89faa9e96cfd1d7420f8))

## [0.0.187](https://github.com/chattocorp/chatto/compare/v0.0.186...v0.0.187) (2026-05-24)


### Features

* **rooms:** seed announcements and general on fresh server boot ([#586](https://github.com/chattocorp/chatto/issues/586)) ([1a82f91](https://github.com/chattocorp/chatto/commit/1a82f918f6a096cc584ebf92ae918b82f34f0c9d))


### Bug Fixes

* **assets:** probe storage backends when Attachment.Storage is missing ([#588](https://github.com/chattocorp/chatto/issues/588)) ([86f7b7c](https://github.com/chattocorp/chatto/commit/86f7b7c1abca4e57064ea63b9cf603b829ca3eb3))

## [0.0.186](https://github.com/chattocorp/chatto/compare/v0.0.185...v0.0.186) (2026-05-24)


### Miscellaneous Chores

* cut release 0.0.186 ([3f6e05e](https://github.com/chattocorp/chatto/commit/3f6e05e9899bb3dff94e7a2bf16f662b59e57b6c))

## [0.0.185](https://github.com/chattocorp/chatto/compare/v0.0.184...v0.0.185) (2026-05-22)


### Bug Fixes

* **migrations:** backfill records for video variants and thumbnails ([#577](https://github.com/chattocorp/chatto/issues/577)) ([ca43ce8](https://github.com/chattocorp/chatto/commit/ca43ce8300101ea679dfc7066c2b588db7a815c0))

## [0.0.184](https://github.com/chattocorp/chatto/compare/v0.0.183...v0.0.184) (2026-05-22)


### Bug Fixes

* **assets:** authorize attachment downloads via canonical Attachment records ([#575](https://github.com/chattocorp/chatto/issues/575)) ([c3ab155](https://github.com/chattocorp/chatto/commit/c3ab155deb72c3c1781457c3773bab7402c2519c))

## [0.0.183](https://github.com/chattocorp/chatto/compare/v0.0.182...v0.0.183) (2026-05-22)


### Features

* **ci:** adopt release-please, retire `mise bump` ([#573](https://github.com/chattocorp/chatto/issues/573)) ([2eb2f67](https://github.com/chattocorp/chatto/commit/2eb2f678ac708316df7f04c3d8592308c7aa1c44))

## 0.0.182

Baseline. History prior to release-please adoption is preserved in git
tags `v0.0.1` … `v0.0.182` and their corresponding GitHub Releases.
