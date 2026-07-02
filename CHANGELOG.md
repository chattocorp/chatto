# Changelog

All notable changes to Chatto. Maintained by release-please from the
conventional-commit messages on `main` — do not edit by hand.

## [0.4.0-beta.11](https://github.com/chattocorp/chatto/compare/v0.4.0-beta.10...v0.4.0-beta.11) (2026-07-02)


### Features

* **api:** add ConnectRPC asset uploads ([#1249](https://github.com/chattocorp/chatto/issues/1249)) ([f97f1d0](https://github.com/chattocorp/chatto/commit/f97f1d097ba887279b228bcb0dd243cfd16f320b))


### Bug Fixes

* **api:** align ConnectRPC permission exposure ([#1246](https://github.com/chattocorp/chatto/issues/1246)) ([cf2eca7](https://github.com/chattocorp/chatto/commit/cf2eca7877b10406f517e64f542fd56d1e73594e))
* **frontend:** clarify echo reply actions ([#1253](https://github.com/chattocorp/chatto/issues/1253)) ([5a2b264](https://github.com/chattocorp/chatto/commit/5a2b2645bd046c3e925bbb2c24c47eecbe534589))
* **frontend:** improve call presence indicators ([#1257](https://github.com/chattocorp/chatto/issues/1257)) ([696a92e](https://github.com/chattocorp/chatto/commit/696a92e008919c2358c188a23963bc9d489fc166))
* **frontend:** reset inline code state when composer clears ([#1251](https://github.com/chattocorp/chatto/issues/1251)) ([0dddeaa](https://github.com/chattocorp/chatto/commit/0dddeaa24e62d028797a93f3cd808e94a1141485))
* **frontend:** restore circular avatars with stable presence dots ([#1252](https://github.com/chattocorp/chatto/issues/1252)) ([14b15b9](https://github.com/chattocorp/chatto/commit/14b15b93382c9b9719b068e889691b5f44f6cf2f))
* **frontend:** use full-width image galleries ([#1247](https://github.com/chattocorp/chatto/issues/1247)) ([f5fe88a](https://github.com/chattocorp/chatto/commit/f5fe88aff3fdfc9cc676dd8735dfd850fc3a7cb3))
* **media:** preserve video aspect ratios ([#1254](https://github.com/chattocorp/chatto/issues/1254)) ([8a85f0a](https://github.com/chattocorp/chatto/commit/8a85f0a434e688fe2a7b25a096c010ee74ebd274))

## [0.4.0-beta.10](https://github.com/chattocorp/chatto/compare/v0.4.0-beta.9...v0.4.0-beta.10) (2026-07-02)


### ⚠ BREAKING CHANGES

* **api:** polish ConnectRPC API for 0.4.0 ([#1224](https://github.com/chattocorp/chatto/issues/1224))

### Features

* **api:** add resource batch reads ([#1232](https://github.com/chattocorp/chatto/issues/1232)) ([8a04ae0](https://github.com/chattocorp/chatto/commit/8a04ae0fa619efc180ff364098f986859f33e041))
* **api:** polish ConnectRPC API for 0.4.0 ([#1224](https://github.com/chattocorp/chatto/issues/1224)) ([06f4361](https://github.com/chattocorp/chatto/commit/06f4361d05e27587839e31b128e38b3ee011c743))
* **core:** store thread follows in EVT ([#1233](https://github.com/chattocorp/chatto/issues/1233)) ([01a2bb3](https://github.com/chattocorp/chatto/commit/01a2bb3d629b83dd30431afcb17e3746a4848d33))
* **dev:** add Mailpit to mise dev ([#1238](https://github.com/chattocorp/chatto/issues/1238)) ([0d07f7e](https://github.com/chattocorp/chatto/commit/0d07f7e8d9540de1d36cf56388f151bd94cb3f2b))
* **frontend:** add multi-image attachment gallery ([#1241](https://github.com/chattocorp/chatto/issues/1241)) ([d8338c5](https://github.com/chattocorp/chatto/commit/d8338c517ef71069a08db44f402b949458ea6e92))
* **frontend:** maximize call pane ([#1240](https://github.com/chattocorp/chatto/issues/1240)) ([7aaa34a](https://github.com/chattocorp/chatto/commit/7aaa34ad4abb9d27cb558b10a8c8944a80240de7))


### Bug Fixes

* **api:** address 0.4.0 surface review findings ([#1228](https://github.com/chattocorp/chatto/issues/1228)) ([bd054ff](https://github.com/chattocorp/chatto/commit/bd054ff0102c3065781064726c1d128f3980700e))
* **api:** close ConnectRPC RBAC gaps ([#1207](https://github.com/chattocorp/chatto/issues/1207)) ([da0b129](https://github.com/chattocorp/chatto/commit/da0b1298db513bdc7a95319535039a01a04010e7))
* **docs:** keep release note cards in grid lanes ([#1204](https://github.com/chattocorp/chatto/issues/1204)) ([a6c79df](https://github.com/chattocorp/chatto/commit/a6c79df79793e9e3927a7d738b4f54ddbc1940f9))
* **frontend:** constrain current user card height ([#1239](https://github.com/chattocorp/chatto/issues/1239)) ([1b536b9](https://github.com/chattocorp/chatto/commit/1b536b96a7d0c7abb6baa152d82d348e0f6b0218))
* **frontend:** defer camera permission until enabled ([#1243](https://github.com/chattocorp/chatto/issues/1243)) ([2145a95](https://github.com/chattocorp/chatto/commit/2145a9535ada73b05a0938b5b6249c264eed99d1))
* **frontend:** improve extreme image thumbnails ([#1227](https://github.com/chattocorp/chatto/issues/1227)) ([d5c596d](https://github.com/chattocorp/chatto/commit/d5c596d56bb306e4503c36d1883900b284d7b5c7))
* **frontend:** localize date formatting ([#1242](https://github.com/chattocorp/chatto/issues/1242)) ([cfc96ec](https://github.com/chattocorp/chatto/commit/cfc96ec847220f580249031d25f5db80dbd89ecf))
* **frontend:** reconcile PWA notification badges ([#1229](https://github.com/chattocorp/chatto/issues/1229)) ([e44645e](https://github.com/chattocorp/chatto/commit/e44645e271cf099eec2e19f9030b10891f76f937))
* **frontend:** show loading state for call media toggles ([#1237](https://github.com/chattocorp/chatto/issues/1237)) ([9063832](https://github.com/chattocorp/chatto/commit/9063832ae074a47852f340b38fa15d755c8399a6))
* **frontend:** style room member search clear button ([#1226](https://github.com/chattocorp/chatto/issues/1226)) ([e43f615](https://github.com/chattocorp/chatto/commit/e43f615e951b12200f1994e844d3b82de4ecdeca))
* **frontend:** wire UI strings to i18n ([#1225](https://github.com/chattocorp/chatto/issues/1225)) ([7eafcd3](https://github.com/chattocorp/chatto/commit/7eafcd34507e6a86e4983ac2ab29c25ee0e6cb95))


### Performance Improvements

* **frontend:** load room members in larger batches ([#1206](https://github.com/chattocorp/chatto/issues/1206)) ([f465a09](https://github.com/chattocorp/chatto/commit/f465a095e88819c6f210f36b1bc334e3c4e06c5a))

## [0.4.0-beta.9](https://github.com/chattocorp/chatto/compare/v0.4.0-beta.8...v0.4.0-beta.9) (2026-06-30)


### Bug Fixes

* **auth:** reject empty-user runtime credentials ([#1201](https://github.com/chattocorp/chatto/issues/1201)) ([43b569c](https://github.com/chattocorp/chatto/commit/43b569c348c89cdf6df1f49a6433b385625a2589))

## [0.4.0-beta.8](https://github.com/chattocorp/chatto/compare/v0.4.0-beta.7...v0.4.0-beta.8) (2026-06-30)


### Features

* **auth:** type runtime credentials ([#1195](https://github.com/chattocorp/chatto/issues/1195)) ([5f0ebe4](https://github.com/chattocorp/chatto/commit/5f0ebe4264d4f4539ce85f4d8c3d1a6a779a9702))


### Bug Fixes

* **frontend:** preserve touch composer line breaks ([#1194](https://github.com/chattocorp/chatto/issues/1194)) ([8c62c70](https://github.com/chattocorp/chatto/commit/8c62c700f1a4a07369cb17ba0dd2ea9141bcdf8d))

## [0.4.0-beta.7](https://github.com/chattocorp/chatto/compare/v0.4.0-beta.6...v0.4.0-beta.7) (2026-06-30)


### ⚠ BREAKING CHANGES

* **operator:** add socket-backed operator user administration ([#1164](https://github.com/chattocorp/chatto/issues/1164))

### Features

* **auth:** add SSO account creation and linking ([#1167](https://github.com/chattocorp/chatto/issues/1167)) ([61723e9](https://github.com/chattocorp/chatto/commit/61723e9e3e6c6f8802558c8a11acab31444c7efb))
* **operator:** add socket-backed operator user administration ([#1164](https://github.com/chattocorp/chatto/issues/1164)) ([6209795](https://github.com/chattocorp/chatto/commit/6209795767fa38e2031bfb77e61b3bcb034a4b77))


### Bug Fixes

* **dockercompose:** enable LiveKit TURN relay ([#1190](https://github.com/chattocorp/chatto/issues/1190)) ([51eb5e7](https://github.com/chattocorp/chatto/commit/51eb5e799f4ebabb395c9f5073219d4015b2ac10))
* **pwa:** reduce service worker reload churn ([#1187](https://github.com/chattocorp/chatto/issues/1187)) ([5489e47](https://github.com/chattocorp/chatto/commit/5489e4742cf577f50295dc8f29d30ed64841245b))

## [0.4.0-beta.6](https://github.com/chattocorp/chatto/compare/v0.4.0-beta.5...v0.4.0-beta.6) (2026-06-29)


### ⚠ BREAKING CHANGES

* **api:** reshape server profile responses ([#1185](https://github.com/chattocorp/chatto/issues/1185))

### Features

* **api:** reshape server profile responses ([#1185](https://github.com/chattocorp/chatto/issues/1185)) ([96bde6e](https://github.com/chattocorp/chatto/commit/96bde6eb3d0ea9b134e7191e41b16fdc07d3bee1))

## [0.4.0-beta.5](https://github.com/chattocorp/chatto/compare/v0.4.0-beta.4...v0.4.0-beta.5) (2026-06-29)


### ⚠ BREAKING CHANGES

* **api:** split ConnectRPC packages ([#1179](https://github.com/chattocorp/chatto/issues/1179))

### Features

* **api:** add ConnectRPC reflection ([#1182](https://github.com/chattocorp/chatto/issues/1182)) ([a93324c](https://github.com/chattocorp/chatto/commit/a93324cf91e21cfab6eb7057f9b35e3545f3cf4c))
* **api:** clean up ConnectRPC surface ([#1171](https://github.com/chattocorp/chatto/issues/1171)) ([03c42af](https://github.com/chattocorp/chatto/commit/03c42af51837bcd999bb3c34989ba706e2d291c5))
* **api:** clean up ConnectRPC surface ([#1178](https://github.com/chattocorp/chatto/issues/1178)) ([b1b6e28](https://github.com/chattocorp/chatto/commit/b1b6e28a818d3f878c0674bd741292d1e33f680e))
* **api:** extract generated TypeScript clients ([#1183](https://github.com/chattocorp/chatto/issues/1183)) ([3480cda](https://github.com/chattocorp/chatto/commit/3480cdab949940d614160897134129693f14e782))
* **api:** extract TypeScript API client ([#1184](https://github.com/chattocorp/chatto/issues/1184)) ([b38b9a5](https://github.com/chattocorp/chatto/commit/b38b9a522cd48b5673109d09007b7d04709b251e))
* **api:** split ConnectRPC packages ([#1179](https://github.com/chattocorp/chatto/issues/1179)) ([6ec286a](https://github.com/chattocorp/chatto/commit/6ec286a469377b5ebe338167cb0244bbc4a9b9d2))
* **docs:** add release notes pages ([#1180](https://github.com/chattocorp/chatto/issues/1180)) ([6418471](https://github.com/chattocorp/chatto/commit/641847194e8d02cd86e8e9827b756a8cec109d56))


### Bug Fixes

* **api:** preserve offline presence in snapshots ([#1172](https://github.com/chattocorp/chatto/issues/1172)) ([7fce244](https://github.com/chattocorp/chatto/commit/7fce244d8f7deecd821966923ce2992c5a656f2c))
* **attachments:** crop extreme image thumbnails ([#1181](https://github.com/chattocorp/chatto/issues/1181)) ([d5dd244](https://github.com/chattocorp/chatto/commit/d5dd244e42ea884cf4739523cda3479a17c1e4f8))
* **messages:** validate reply targets before posting ([#1176](https://github.com/chattocorp/chatto/issues/1176)) ([2919a1a](https://github.com/chattocorp/chatto/commit/2919a1a4fcb0cf5b13a6e22764329bee0f9f1d1d))

## [0.4.0-beta.4](https://github.com/chattocorp/chatto/compare/v0.4.0-beta.3...v0.4.0-beta.4) (2026-06-28)


### ⚠ BREAKING CHANGES

* **api:** replace GraphQL with ConnectRPC ([#1166](https://github.com/chattocorp/chatto/issues/1166))

### Features

* **api:** add ConnectRPC DM start ([#1157](https://github.com/chattocorp/chatto/issues/1157)) ([c46ef79](https://github.com/chattocorp/chatto/commit/c46ef79ce782fad2f9cd26cb4db42fd7ae581a30))
* **api:** add protobuf realtime websocket ([#1158](https://github.com/chattocorp/chatto/issues/1158)) ([9e8e34c](https://github.com/chattocorp/chatto/commit/9e8e34cdc778be86007d0f6596468b445cfa4a0e))
* **api:** replace GraphQL with ConnectRPC ([#1166](https://github.com/chattocorp/chatto/issues/1166)) ([3dd3fa6](https://github.com/chattocorp/chatto/commit/3dd3fa686fc3c89912dcdf02475578389608f627))
* **config:** configure SMTP TLS verification ([#1159](https://github.com/chattocorp/chatto/issues/1159)) ([1f5c8b0](https://github.com/chattocorp/chatto/commit/1f5c8b09d2f4c13d0c13825c38e2bb5c4807beeb))
* **connectrpc:** add message management API ([#1146](https://github.com/chattocorp/chatto/issues/1146)) ([c07b049](https://github.com/chattocorp/chatto/commit/c07b0497ab09ae970895809edb5b31fd79c5e093))
* **connectrpc:** add room directory service ([#1138](https://github.com/chattocorp/chatto/issues/1138)) ([c1f13cf](https://github.com/chattocorp/chatto/commit/c1f13cfb4d0dc9cacb019c430db4f8494026ed02))
* **connectrpc:** add room lifecycle service ([#1134](https://github.com/chattocorp/chatto/issues/1134)) ([3f2b3a9](https://github.com/chattocorp/chatto/commit/3f2b3a922f97c4f99f20913e4e4d4a944bb79704))
* **frontend:** refresh admin system dashboard ([#1160](https://github.com/chattocorp/chatto/issues/1160)) ([5c54899](https://github.com/chattocorp/chatto/commit/5c54899f1eb676cff77ca3707b9e98eb36b639c6))
* **frontend:** send typing indicators with ConnectRPC ([#1155](https://github.com/chattocorp/chatto/issues/1155)) ([1a131ee](https://github.com/chattocorp/chatto/commit/1a131eea08bb32a89462bbd0c010617cc2fdaedb))
* **frontend:** use ConnectRPC for message writes ([#1153](https://github.com/chattocorp/chatto/issues/1153)) ([4b34f34](https://github.com/chattocorp/chatto/commit/4b34f341f4e96adb87d775c5ea2fc0ae04e12aee))
* **frontend:** use ConnectRPC for room commands ([#1150](https://github.com/chattocorp/chatto/issues/1150)) ([bfff68e](https://github.com/chattocorp/chatto/commit/bfff68e8d48a2adbd512be249e9482c467b03a88))


### Bug Fixes

* **api:** centralize Connect room RBAC in core ([#1149](https://github.com/chattocorp/chatto/issues/1149)) ([8ba5b0c](https://github.com/chattocorp/chatto/commit/8ba5b0c2a3854f1ca7f18084a3225661a5e3d205))
* **ci:** gate release-please on green ci ([#1135](https://github.com/chattocorp/chatto/issues/1135)) ([4decb0f](https://github.com/chattocorp/chatto/commit/4decb0f1362e876e461ce9436a6ce0f8cb340eab))
* **frontend:** address svelte guidance review ([#1154](https://github.com/chattocorp/chatto/issues/1154)) ([d8c4010](https://github.com/chattocorp/chatto/commit/d8c4010b1b02ec4b65a15408b07f3800180a2a5e))
* **frontend:** make scrollbars follow selected theme ([#1152](https://github.com/chattocorp/chatto/issues/1152)) ([9c5fa16](https://github.com/chattocorp/chatto/commit/9c5fa16da9555d38c0331e5876d4b35b025d4371))
* **frontend:** refresh messages after local deletions ([#1148](https://github.com/chattocorp/chatto/issues/1148)) ([cefc22a](https://github.com/chattocorp/chatto/commit/cefc22a77efee0f333b848a054c0a56078b0a0d6))
* **frontend:** restyle reply attribution preview ([#1140](https://github.com/chattocorp/chatto/issues/1140)) ([909c1f4](https://github.com/chattocorp/chatto/commit/909c1f4a2d67ba2979be765b9eaecff611e96e90))

## [0.4.0-beta.3](https://github.com/chattocorp/chatto/compare/v0.4.0-beta.2...v0.4.0-beta.3) (2026-06-25)


### ⚠ BREAKING CHANGES

* **api:** use optional timeline presence fields ([#1110](https://github.com/chattocorp/chatto/issues/1110))

### Features

* **api:** migrate reactions to ConnectRPC ([#1128](https://github.com/chattocorp/chatto/issues/1128)) ([161f51c](https://github.com/chattocorp/chatto/commit/161f51ccb4cc0cd3b1b098d1b5aa41c3f4405c8d))
* **api:** use optional timeline presence fields ([#1110](https://github.com/chattocorp/chatto/issues/1110)) ([5c1406f](https://github.com/chattocorp/chatto/commit/5c1406f0a28502be869964c87561c0e107c81446))
* **presence:** add user-controlled presence modes ([#1095](https://github.com/chattocorp/chatto/issues/1095)) ([9e8f696](https://github.com/chattocorp/chatto/commit/9e8f696df7dc2489c639479f01eb7269ba13a922))


### Bug Fixes

* **api:** make ConnectRPC plumbing idiomatic ([#1123](https://github.com/chattocorp/chatto/issues/1123)) ([338f573](https://github.com/chattocorp/chatto/commit/338f57315cf611518ff4570434ee7faae1ccab7d))
* **api:** tighten ConnectRPC caller auth ([#1126](https://github.com/chattocorp/chatto/issues/1126)) ([bb8c10d](https://github.com/chattocorp/chatto/commit/bb8c10df48a2c7e8a9a94164ee66d24d0517ac31))
* **connectapi:** harden timeline and thread read handling ([#1117](https://github.com/chattocorp/chatto/issues/1117)) ([ba027fe](https://github.com/chattocorp/chatto/commit/ba027fe3b7727620307bc4936633effe8abd255d))
* **connectrpc:** cap request message size ([#1102](https://github.com/chattocorp/chatto/issues/1102)) ([a773531](https://github.com/chattocorp/chatto/commit/a773531e687de72645ee78b1aa09f07f9d61ef61))
* **connectrpc:** reject missing read anchors ([#1109](https://github.com/chattocorp/chatto/issues/1109)) ([f2f68b9](https://github.com/chattocorp/chatto/commit/f2f68b96fca00c177975600f1e9f38f2787a3c4b))
* **core:** complete service inventory metrics ([#1130](https://github.com/chattocorp/chatto/issues/1130)) ([9bc89f3](https://github.com/chattocorp/chatto/commit/9bc89f3e116df73330be22484b13a999419b12ed))
* **core:** prevent read marker regressions ([#1107](https://github.com/chattocorp/chatto/issues/1107)) ([cb81d58](https://github.com/chattocorp/chatto/commit/cb81d583f9c789319790109624af5ad8d112d680))
* **frontend:** clarify remote push notification support ([#1105](https://github.com/chattocorp/chatto/issues/1105)) ([bfdbdea](https://github.com/chattocorp/chatto/commit/bfdbdea4050d529ba060f5931009d74026a8631f))
* **frontend:** submit simple message edits with enter ([#1129](https://github.com/chattocorp/chatto/issues/1129)) ([f5651b4](https://github.com/chattocorp/chatto/commit/f5651b4413b70aaa954d3bdb7c553df21e7c42ca))
* **frontend:** sync room thread follow bell state ([#1121](https://github.com/chattocorp/chatto/issues/1121)) ([4048f23](https://github.com/chattocorp/chatto/commit/4048f23256f87e417509fb887d2919c59bad5a38))


### Performance Improvements

* **build:** improve frontend and CLI cache reuse ([#1106](https://github.com/chattocorp/chatto/issues/1106)) ([f22da3a](https://github.com/chattocorp/chatto/commit/f22da3adcd5a8affe8b15715cd02569baddad2e7))
* **frontend:** split chat code from app chrome ([#1103](https://github.com/chattocorp/chatto/issues/1103)) ([4a4a4de](https://github.com/chattocorp/chatto/commit/4a4a4de0747e73d37183bc3fde89f6d0f45c8890))

## [0.4.0-beta.2](https://github.com/chattocorp/chatto/compare/v0.4.0-beta.1...v0.4.0-beta.2) (2026-06-24)


### Features

* **api:** port message posting to ConnectRPC ([#1093](https://github.com/chattocorp/chatto/issues/1093)) ([011018b](https://github.com/chattocorp/chatto/commit/011018bab165ba29e310f2e527a6dae9648899e2))
* **api:** port read state and thread follow to ConnectRPC ([#1087](https://github.com/chattocorp/chatto/issues/1087)) ([f2128d6](https://github.com/chattocorp/chatto/commit/f2128d60d6d1706217f06566102788900619e053))
* **connectrpc:** port thread history reads ([#1083](https://github.com/chattocorp/chatto/issues/1083)) ([4b81b4d](https://github.com/chattocorp/chatto/commit/4b81b4dbf78e879cdf2b10060f3777f6d2071dc3))
* **frontend:** add Paraglide-based client-shell i18n ([#1077](https://github.com/chattocorp/chatto/issues/1077)) ([1a4ab07](https://github.com/chattocorp/chatto/commit/1a4ab07211482af1236b3921607fd2deb8746f4f))
* **frontend:** move UI strings into i18n catalogs ([#1084](https://github.com/chattocorp/chatto/issues/1084)) ([d310382](https://github.com/chattocorp/chatto/commit/d310382e0795007da388e0514ac7d2056e961898))
* **profile:** add custom user statuses ([#1081](https://github.com/chattocorp/chatto/issues/1081)) ([1d1d7d2](https://github.com/chattocorp/chatto/commit/1d1d7d214a28b9c9eb38c50522e44b943d7e5cb5))


### Bug Fixes

* **api:** include user status in generated docs ([#1092](https://github.com/chattocorp/chatto/issues/1092)) ([52521fa](https://github.com/chattocorp/chatto/commit/52521fa5eeff94d9bebffabb010a6eb4b5e9de78))
* **connectapi:** harden message post migration ([#1097](https://github.com/chattocorp/chatto/issues/1097)) ([b15fb14](https://github.com/chattocorp/chatto/commit/b15fb14c2ee708915ab79255f6a86aab3c4cc764))
* **frontend:** align call control button colors ([#1085](https://github.com/chattocorp/chatto/issues/1085)) ([4b7f37e](https://github.com/chattocorp/chatto/commit/4b7f37e87d1bcfe8b388f59aa1ae70b7e3aff5ea))
* **frontend:** defer unread separator until return to the room ([#1079](https://github.com/chattocorp/chatto/issues/1079)) ([9535694](https://github.com/chattocorp/chatto/commit/95356945a66376560017888ef0291295f6d13f1e))
* **frontend:** improve unread channel contrast ([#1089](https://github.com/chattocorp/chatto/issues/1089)) ([74247b4](https://github.com/chattocorp/chatto/commit/74247b42833d07c33a2950dc357cf5c4b06a3f66))

## [0.4.0-beta.1](https://github.com/chattocorp/chatto/compare/v0.3.8...v0.4.0-beta.1) (2026-06-23)


### Features

* add universal rooms ([#1046](https://github.com/chattocorp/chatto/issues/1046)) ([0b8c5cb](https://github.com/chattocorp/chatto/commit/0b8c5cb839876416a8262260ddc6a051ee0c94ba))
* **admin:** filter event log ([#1056](https://github.com/chattocorp/chatto/issues/1056)) ([d8bd280](https://github.com/chattocorp/chatto/commit/d8bd28076112e4e2a1488190cb29e9bf0acbc5cc))
* **api:** add ConnectRPC public API PoC ([#1067](https://github.com/chattocorp/chatto/issues/1067)) ([7aeb8f7](https://github.com/chattocorp/chatto/commit/7aeb8f7fd629da040d2e916600215fe3d02d0f26))
* **api:** add ConnectRPC room timeline PoC ([#1074](https://github.com/chattocorp/chatto/issues/1074)) ([920fcaa](https://github.com/chattocorp/chatto/commit/920fcaa26ca577ada529e2e1ef19d041d5baa47f))
* **core:** persist link preview assets via storage backend ([#1060](https://github.com/chattocorp/chatto/issues/1060)) ([005deb1](https://github.com/chattocorp/chatto/commit/005deb1365f1899176cca57f91db8265cf7da009))
* **exporter:** add deployment-wide prometheus exporter ([#1059](https://github.com/chattocorp/chatto/issues/1059)) ([5aa29c7](https://github.com/chattocorp/chatto/commit/5aa29c747babe5b4dacc12a9a63eef57bcf36ec8))
* **frontend:** consolidate frontend design system ([#1053](https://github.com/chattocorp/chatto/issues/1053)) ([7fc39ab](https://github.com/chattocorp/chatto/commit/7fc39ab6aebdba74bd8eef56ba05323bf60ad901))
* **frontend:** improve admin member details ([#1057](https://github.com/chattocorp/chatto/issues/1057)) ([8c8ccce](https://github.com/chattocorp/chatto/commit/8c8cccee5335bf2d10948414a65b2d75a547c30f))
* **frontend:** show call participants in room sidebar ([#1036](https://github.com/chattocorp/chatto/issues/1036)) ([8cd0858](https://github.com/chattocorp/chatto/commit/8cd085877d44633aa54578abf2d50a62942c0085))
* **frontend:** show reaction names in popups ([#1044](https://github.com/chattocorp/chatto/issues/1044)) ([e141b74](https://github.com/chattocorp/chatto/commit/e141b7441ca7d8d62252f2a9376ca3f2a768ea9d))
* **frontend:** show room descriptions in header ([#1037](https://github.com/chattocorp/chatto/issues/1037)) ([44f9c67](https://github.com/chattocorp/chatto/commit/44f9c67c979535584c12838ccc46eaf40a879d6c))


### Bug Fixes

* **auth:** add structured unauthenticated GraphQL errors ([#1048](https://github.com/chattocorp/chatto/issues/1048)) ([510c07d](https://github.com/chattocorp/chatto/commit/510c07dd38ad3ccc9e87f515878c96594c72c9dd))
* **frontend:** align muted call participant icon ([#1050](https://github.com/chattocorp/chatto/issues/1050)) ([68cea04](https://github.com/chattocorp/chatto/commit/68cea040f6129134b50cf1c745274e3f669b3746))
* **frontend:** harden asset proxy token handling ([#1054](https://github.com/chattocorp/chatto/issues/1054)) ([8797c65](https://github.com/chattocorp/chatto/commit/8797c65aa35b304ac5e77216f783f404865d2928))
* **frontend:** ignore stale DM member loads when switching rooms ([#1065](https://github.com/chattocorp/chatto/issues/1065)) ([b4264b7](https://github.com/chattocorp/chatto/commit/b4264b77c12b4492b0391597072e20a1809b0316))
* **frontend:** reconcile notification badge dismissals ([#1058](https://github.com/chattocorp/chatto/issues/1058)) ([13c7a6e](https://github.com/chattocorp/chatto/commit/13c7a6ef51a34f6a99964fcbe167f30fd8e7d304))
* **frontend:** remove redundant universal room badge ([#1052](https://github.com/chattocorp/chatto/issues/1052)) ([5f6131e](https://github.com/chattocorp/chatto/commit/5f6131ee3fe98e5713a2eb64e2da22f5d5287e68))
* **frontend:** restrict same-tab message links ([#1068](https://github.com/chattocorp/chatto/issues/1068)) ([d43d23f](https://github.com/chattocorp/chatto/commit/d43d23f70da28a324743673f585085c70f5d89ac))
* **notifications:** preserve unread badge state across dismissals ([#1069](https://github.com/chattocorp/chatto/issues/1069)) ([03444e3](https://github.com/chattocorp/chatto/commit/03444e39cf171bb87277d6db20fd20d422378a3d))
* **voice:** scope LiveKit observations to active calls ([#1049](https://github.com/chattocorp/chatto/issues/1049)) ([dcd95c8](https://github.com/chattocorp/chatto/commit/dcd95c8cdd9f964e36eeea73592d2827dcb83c9e))

## [0.3.8](https://github.com/chattocorp/chatto/compare/v0.3.7...v0.3.8) (2026-06-20)


### Bug Fixes

* downgrade invalid session cookie logs ([#1029](https://github.com/chattocorp/chatto/issues/1029)) ([5bbbe88](https://github.com/chattocorp/chatto/commit/5bbbe88a5f34f885266c8afcf66cff6762adc6ca))
* improve push notification routing ([#1031](https://github.com/chattocorp/chatto/issues/1031)) ([bda7d3d](https://github.com/chattocorp/chatto/commit/bda7d3da31a1e02158fa3cc6646ff4c1d6cb59f8))
* **sidebar:** server-local sidebar links now open in the same window ([#1041](https://github.com/chattocorp/chatto/issues/1041)) ([b206d56](https://github.com/chattocorp/chatto/commit/b206d56dfde6ecfd9f3e82a32134c8685245a2f4))


### Performance Improvements

* add opt-in profiling diagnostics ([#1038](https://github.com/chattocorp/chatto/issues/1038)) ([ca2a2f6](https://github.com/chattocorp/chatto/commit/ca2a2f69efe049e85dc3e18c8c9d2f1a92cd6ad3))
* fast-path projection stream sequence parsing ([#1042](https://github.com/chattocorp/chatto/issues/1042)) ([ad28708](https://github.com/chattocorp/chatto/commit/ad28708ea90a0e8eb4b69bbb3faf51abf7ee41a5))
* optimize projection dispatch matching ([#1040](https://github.com/chattocorp/chatto/issues/1040)) ([8f40573](https://github.com/chattocorp/chatto/commit/8f40573bf1d3b7107be3d99ca61c51738f9c1afd))
* optimize projection replay and memory ([#1032](https://github.com/chattocorp/chatto/issues/1032)) ([f0118ed](https://github.com/chattocorp/chatto/commit/f0118eda47250f1df50a744ab3fb4e9f5774497d))
* replay projections through shared EVT fanout ([#1035](https://github.com/chattocorp/chatto/issues/1035)) ([15d322d](https://github.com/chattocorp/chatto/commit/15d322db9ab01012129f75911b98e6a83cac0815))

## [0.3.7](https://github.com/chattocorp/chatto/compare/v0.3.6...v0.3.7) (2026-06-19)


### Bug Fixes

* remove graphql error logging ([#1026](https://github.com/chattocorp/chatto/issues/1026)) ([bb3071c](https://github.com/chattocorp/chatto/commit/bb3071c3eb2acc63fb4e7c1fc655824e9fce0878))

## [0.3.6](https://github.com/chattocorp/chatto/compare/v0.3.5...v0.3.6) (2026-06-19)


### Performance Improvements

* reduce room timeline projection retention ([#1016](https://github.com/chattocorp/chatto/issues/1016)) ([dd779b7](https://github.com/chattocorp/chatto/commit/dd779b7752fea58c0383fe81cec60a6689a8da35))

## [0.3.5](https://github.com/chattocorp/chatto/compare/v0.3.4...v0.3.5) (2026-06-19)


### Features

* add LiveKit screen sharing ([#1021](https://github.com/chattocorp/chatto/issues/1021)) ([068abda](https://github.com/chattocorp/chatto/commit/068abda7cf55df077ac0d7a78b6912c2bba9fc63))
* **frontend:** add call join leave sound cues ([#1023](https://github.com/chattocorp/chatto/issues/1023)) ([1cf9e85](https://github.com/chattocorp/chatto/commit/1cf9e850bc8b48cc46ae6eea36be416940e16e6c))
* **frontend:** add display theme preference ([#1018](https://github.com/chattocorp/chatto/issues/1018)) ([ed7e276](https://github.com/chattocorp/chatto/commit/ed7e2767e5284144cdaa0ee923a1ca7f91af5f43))


### Bug Fixes

* **calls:** improve LiveKit join resilience ([#1022](https://github.com/chattocorp/chatto/issues/1022)) ([e9a0e55](https://github.com/chattocorp/chatto/commit/e9a0e55dcbfa75c783d174530de6771bf98f5313))
* **frontend:** make thread badges native links ([#1020](https://github.com/chattocorp/chatto/issues/1020)) ([e8c3642](https://github.com/chattocorp/chatto/commit/e8c364242624a9412aef63c0e93508bb9ed2074b))
* hide call lifecycle events from room history ([#1017](https://github.com/chattocorp/chatto/issues/1017)) ([5315770](https://github.com/chattocorp/chatto/commit/53157702aba589e58f5e5580214187f636ed0dff))

## [0.3.4](https://github.com/chattocorp/chatto/compare/v0.3.3...v0.3.4) (2026-06-19)


### Features

* add scoped server sign-out ([#1006](https://github.com/chattocorp/chatto/issues/1006)) ([1fc081b](https://github.com/chattocorp/chatto/commit/1fc081b0189b5d60313fbe496a93166b68cbaa06))
* **frontend:** refresh call sidebar UI ([#1001](https://github.com/chattocorp/chatto/issues/1001)) ([cd48c1a](https://github.com/chattocorp/chatto/commit/cd48c1aa8dcf6357d939a4442923bc443284dfb4))


### Bug Fixes

* **frontend:** clear stale mention autocomplete state ([#1015](https://github.com/chattocorp/chatto/issues/1015)) ([9132ab6](https://github.com/chattocorp/chatto/commit/9132ab68f5a5fd69b7c4ea16e47dc3f8e5396cf6))
* **frontend:** eagerly load room members ([#1009](https://github.com/chattocorp/chatto/issues/1009)) ([d76ae9a](https://github.com/chattocorp/chatto/commit/d76ae9ae4d1f66aeef60fb07687a1a0aafd73535))
* **frontend:** prevent room badge clipping ([#1012](https://github.com/chattocorp/chatto/issues/1012)) ([5c86be7](https://github.com/chattocorp/chatto/commit/5c86be751a41d2ec6eca69f3eba6ffc4b7579c99))
* reconcile in-app notification badges ([#1008](https://github.com/chattocorp/chatto/issues/1008)) ([be8cb02](https://github.com/chattocorp/chatto/commit/be8cb02fa6045470940a4a58532858c41e19c633))


### Performance Improvements

* share projection event consumers ([#1011](https://github.com/chattocorp/chatto/issues/1011)) ([31e08fc](https://github.com/chattocorp/chatto/commit/31e08fc4f76a324e0518d94ebf9cf06c36979821))

## [0.3.3](https://github.com/chattocorp/chatto/compare/v0.3.2...v0.3.3) (2026-06-19)


### Performance Improvements

* optimize projection startup paths ([#1005](https://github.com/chattocorp/chatto/issues/1005)) ([b69f2ef](https://github.com/chattocorp/chatto/commit/b69f2ef93c3263a2021a75b71e2d131de28ab2ac))

## [0.3.2](https://github.com/chattocorp/chatto/compare/v0.3.1...v0.3.2) (2026-06-19)


### Features

* monitor projection startup duration ([#1004](https://github.com/chattocorp/chatto/issues/1004)) ([3c6083c](https://github.com/chattocorp/chatto/commit/3c6083ca095ea8a3ce6dd86850f97ec3014b64d7))


### Bug Fixes

* **frontend:** preserve nested reply quotes ([#1000](https://github.com/chattocorp/chatto/issues/1000)) ([5f97896](https://github.com/chattocorp/chatto/commit/5f978963d1d203c210c3c8d4002da3dd86130560))
* **graphql:** enforce room move group permissions ([#987](https://github.com/chattocorp/chatto/issues/987)) ([1364b7b](https://github.com/chattocorp/chatto/commit/1364b7b4752a5b13a26752027d19d8cdae4a9764))

## [0.3.1](https://github.com/chattocorp/chatto/compare/v0.3.0...v0.3.1) (2026-06-18)


### Features

* quote selected text when replying ([#978](https://github.com/chattocorp/chatto/issues/978)) ([4844e89](https://github.com/chattocorp/chatto/commit/4844e89d62c3ca569960c3817236abe4d29699ce))


### Bug Fixes

* correct push notification deep links ([#982](https://github.com/chattocorp/chatto/issues/982)) ([d6bfe9f](https://github.com/chattocorp/chatto/commit/d6bfe9fa9cff5d9522ef9120a5a452bbb93248f6))
* **frontend:** add embed frame vertical spacing ([#976](https://github.com/chattocorp/chatto/issues/976)) ([4137f7f](https://github.com/chattocorp/chatto/commit/4137f7fa4d6310032363e4c75e6659b7babedbac))
* **frontend:** echo local room posts after send ([#980](https://github.com/chattocorp/chatto/issues/980)) ([33f0f46](https://github.com/chattocorp/chatto/commit/33f0f46135318ee916c8acda68d6c0debf8af53f))
* **frontend:** remove server name from room header ([#979](https://github.com/chattocorp/chatto/issues/979)) ([5e58bd5](https://github.com/chattocorp/chatto/commit/5e58bd5ee07d7c3a882feaeb8ba7eefab4e6931f))
* **frontend:** tighten mobile message action sheet ([#981](https://github.com/chattocorp/chatto/issues/981)) ([e30a153](https://github.com/chattocorp/chatto/commit/e30a15301181f5387b917af9bd6dd94e5246a0ce))

## [0.3.0](https://github.com/chattocorp/chatto/compare/v0.2.3...v0.3.0) (2026-06-18)


### ⚠ BREAKING CHANGES

* **sidebar:** list rooms visible via room.list ([#961](https://github.com/chattocorp/chatto/issues/961))

### Features

* add simple and rich composer modes ([#974](https://github.com/chattocorp/chatto/issues/974)) ([ec5bcea](https://github.com/chattocorp/chatto/commit/ec5bceaaba4f87c162366ed1a98b95b622041f95))
* gate message attachments with message.attach ([#966](https://github.com/chattocorp/chatto/issues/966)) ([2870f0f](https://github.com/chattocorp/chatto/commit/2870f0faa0b12c0d8b618a7bacaf4f2a8fce2e49))
* improve linked message previews ([#970](https://github.com/chattocorp/chatto/issues/970)) ([aecdb1b](https://github.com/chattocorp/chatto/commit/aecdb1b3b1762b44ac21e9a62fab0d1a462a2b99))
* improve room member loading and search ([#963](https://github.com/chattocorp/chatto/issues/963)) ([33bd45a](https://github.com/chattocorp/chatto/commit/33bd45a75949fa2c448d3c8625f375c855233e7f))
* **messages:** add copy link menu action ([#969](https://github.com/chattocorp/chatto/issues/969)) ([2afdee2](https://github.com/chattocorp/chatto/commit/2afdee20780d30aee9a6c8018c4f77e6f3d388dd))
* **sidebar:** list rooms visible via room.list ([#961](https://github.com/chattocorp/chatto/issues/961)) ([fe27c06](https://github.com/chattocorp/chatto/commit/fe27c068a834762f79c61e6a480907345ba89b58))
* simplify web push opt-in ([#971](https://github.com/chattocorp/chatto/issues/971)) ([6abb0ce](https://github.com/chattocorp/chatto/commit/6abb0ce1993618c39fc3d85ba3639e9be5348998))


### Bug Fixes

* **composer:** preserve trailing hashes in headings ([#967](https://github.com/chattocorp/chatto/issues/967)) ([3028cb2](https://github.com/chattocorp/chatto/commit/3028cb215a09d15f2ac5ed2216377f4d20ed9484))
* **frontend:** align chat control border radii ([#968](https://github.com/chattocorp/chatto/issues/968)) ([5bc44df](https://github.com/chattocorp/chatto/commit/5bc44df8e4316d57437088bc988de11b8d7d8692))
* **frontend:** improve blockquote styling ([#973](https://github.com/chattocorp/chatto/issues/973)) ([441706c](https://github.com/chattocorp/chatto/commit/441706c0385a84cb6df6cb4657f2572088e5f798))
* **frontend:** route room badges from scoped notifications ([#972](https://github.com/chattocorp/chatto/issues/972)) ([8bb1cc1](https://github.com/chattocorp/chatto/commit/8bb1cc1c6e5d44f1954b6e1532312ca03000b072))
* tighten sidebar item spacing ([#975](https://github.com/chattocorp/chatto/issues/975)) ([8aab581](https://github.com/chattocorp/chatto/commit/8aab581c698e6468d2071bbae2c862d50b8a649b))

## [0.2.3](https://github.com/chattocorp/chatto/compare/v0.2.2...v0.2.3) (2026-06-18)


### Features

* add notification sound shaping controls ([#962](https://github.com/chattocorp/chatto/issues/962)) ([585fa4b](https://github.com/chattocorp/chatto/commit/585fa4b48b058e8b0c411306815ec567a4a421b9))
* **composer:** submit with Ctrl/Cmd+Enter ([#960](https://github.com/chattocorp/chatto/issues/960)) ([461f911](https://github.com/chattocorp/chatto/commit/461f9114e33fca7bae13ac324925a928594a5d08))


### Bug Fixes

* **composer:** keep autolink boundaries editable ([#964](https://github.com/chattocorp/chatto/issues/964)) ([2170f5f](https://github.com/chattocorp/chatto/commit/2170f5f1781396a7a24defa83f667a112f6d4a52))
* **frontend:** restore push notification routing ([#957](https://github.com/chattocorp/chatto/issues/957)) ([b000610](https://github.com/chattocorp/chatto/commit/b000610da536dc26cdb5861226c6f025c1ef9647))
* support configurable Docker runtime user ([#959](https://github.com/chattocorp/chatto/issues/959)) ([edb4595](https://github.com/chattocorp/chatto/commit/edb459508b7458b08c295ac30016f000f74a3e7d))

## [0.2.2](https://github.com/chattocorp/chatto/compare/v0.2.1...v0.2.2) (2026-06-17)


### Features

* group room files by date ([#937](https://github.com/chattocorp/chatto/issues/937)) ([b13674b](https://github.com/chattocorp/chatto/commit/b13674b8a13492ae361c870b886e2fccb2456edf))
* **sidebar:** add group sidebar links ([#915](https://github.com/chattocorp/chatto/issues/915)) ([aea26da](https://github.com/chattocorp/chatto/commit/aea26da20ef0ee7afc86021e3671eaafcd67be7f))


### Bug Fixes

* log graphql errors ([#955](https://github.com/chattocorp/chatto/issues/955)) ([692bfc9](https://github.com/chattocorp/chatto/commit/692bfc95c5179ddcc869d0f154094ef226c6718c))
* represent deleted room members ([#934](https://github.com/chattocorp/chatto/issues/934)) ([91ad1dc](https://github.com/chattocorp/chatto/commit/91ad1dc2047b572df6097296ac533dc22e02b285))

## [0.2.1](https://github.com/chattocorp/chatto/compare/v0.2.0...v0.2.1) (2026-06-17)


### Features

* add room files sidebar ([#920](https://github.com/chattocorp/chatto/issues/920)) ([23e3415](https://github.com/chattocorp/chatto/commit/23e34154e899e0aeadcaa46118914f6966a6221c))
* **cli:** remove reset command ([60502e3](https://github.com/chattocorp/chatto/commit/60502e3fe11ae70943abf2c0856ab1496314349d))
* **cli:** remove reset command ([#928](https://github.com/chattocorp/chatto/issues/928)) ([3380efd](https://github.com/chattocorp/chatto/commit/3380efd91579f3c115f2d5918be14d8aa88cdd4c))


### Bug Fixes

* **e2e:** wait for posted message articles ([#923](https://github.com/chattocorp/chatto/issues/923)) ([c7d9e22](https://github.com/chattocorp/chatto/commit/c7d9e22a462e9f0f3f21762bfb9f6fc8f3155d79))
* **frontend:** confirm mention autocomplete with enter ([d28aa4e](https://github.com/chattocorp/chatto/commit/d28aa4e72d44d2cb480a06045ff215d61e87f2db))
* **frontend:** use app modal for mention confirmation ([#927](https://github.com/chattocorp/chatto/issues/927)) ([f7ff517](https://github.com/chattocorp/chatto/commit/f7ff5173bde71422a3dc45c72ac1268b91924941))
* tolerate stale room members ([#932](https://github.com/chattocorp/chatto/issues/932)) ([40c7d6c](https://github.com/chattocorp/chatto/commit/40c7d6cc0c0847764b8c02592197ee8f14657349))
* update thread replies after send ([#924](https://github.com/chattocorp/chatto/issues/924)) ([2062fdc](https://github.com/chattocorp/chatto/commit/2062fdc9f8686f44a181780b3692364b266ff65b))

## [0.2.0](https://github.com/chattocorp/chatto/compare/v0.1.0...v0.2.0) (2026-06-17)


### ⚠ BREAKING CHANGES

* **docker:** use config and data root paths ([#903](https://github.com/chattocorp/chatto/issues/903))

### Features

* add notification badge counts ([#909](https://github.com/chattocorp/chatto/issues/909)) ([f25a69d](https://github.com/chattocorp/chatto/commit/f25a69da861628ebcb3a07ca1cbc1d9e2744fcf4))
* **auth:** configure email OTP throttling ([#902](https://github.com/chattocorp/chatto/issues/902)) ([8c2d202](https://github.com/chattocorp/chatto/commit/8c2d2024b7e76df74fe3305736fa7f9683c353ac))
* **frontend:** preview Markdown in composer ([#876](https://github.com/chattocorp/chatto/issues/876)) ([06afedb](https://github.com/chattocorp/chatto/commit/06afedbc7d1662d3793c549a402bc3343eb9e37d))
* show room sidebar in DMs ([#912](https://github.com/chattocorp/chatto/issues/912)) ([32222fa](https://github.com/chattocorp/chatto/commit/32222fa82766060eb1b645fb507e1ea1ec1f2b19))


### Bug Fixes

* **auth:** make CSRF tokens stateless ([#900](https://github.com/chattocorp/chatto/issues/900)) ([a2da80c](https://github.com/chattocorp/chatto/commit/a2da80c478700c163240c3c5a816386b1d58c78f))
* **ci:** checkout docs image PR refs ([#906](https://github.com/chattocorp/chatto/issues/906)) ([a2af9a2](https://github.com/chattocorp/chatto/commit/a2af9a294946aecea76cb121d66ed21f220bc11b))
* **docker:** use config and data root paths ([#903](https://github.com/chattocorp/chatto/issues/903)) ([c90f0d9](https://github.com/chattocorp/chatto/commit/c90f0d9a4ee0711f16143cb28904dc7623ef39c6))
* **frontend:** remount room on notification switch ([#908](https://github.com/chattocorp/chatto/issues/908)) ([fcba838](https://github.com/chattocorp/chatto/commit/fcba83843711a568e0356518bd25e78fe06835b8))
* **frontend:** show active call badges for DMs ([#899](https://github.com/chattocorp/chatto/issues/899)) ([a7299e1](https://github.com/chattocorp/chatto/commit/a7299e15978c6b03ccd10889dc27d04e483851ad))
* refresh room layout state after room creation ([#907](https://github.com/chattocorp/chatto/issues/907)) ([7cd94d2](https://github.com/chattocorp/chatto/commit/7cd94d27c86fcc09f669e36bfc92031271785633))
* support implicit SMTP TLS ([#905](https://github.com/chattocorp/chatto/issues/905)) ([d7d83b1](https://github.com/chattocorp/chatto/commit/d7d83b1a98bf6bcf199776e188f9647b9c23cf78))
* tidy server lifecycle logs ([#914](https://github.com/chattocorp/chatto/issues/914)) ([2b95bf4](https://github.com/chattocorp/chatto/commit/2b95bf42c1687ad8c2c3a91c589c68084eb2be5f))

## [0.1.0](https://github.com/chattocorp/chatto/compare/v0.1.0-rc.0...v0.1.0) (2026-06-16)


### Features

* **auth:** use bearer tokens for origin GraphQL ([#897](https://github.com/chattocorp/chatto/issues/897)) ([cf9b552](https://github.com/chattocorp/chatto/commit/cf9b55294fd0b17636a181a35cb84ac9699ea85a))


### Bug Fixes

* **frontend:** keep sidebars visible on fresh sessions ([#891](https://github.com/chattocorp/chatto/issues/891)) ([1cb5717](https://github.com/chattocorp/chatto/commit/1cb571721e7ead02ca8cfd12d961937ad5f648fb))
* **frontend:** remember last visited DM rooms ([#894](https://github.com/chattocorp/chatto/issues/894)) ([de8efb0](https://github.com/chattocorp/chatto/commit/de8efb0f8a827d4f9e40c103fe429d4e7674fb8e))

## [0.1.0-rc.0](https://github.com/chattocorp/chatto/compare/v0.1.0-beta.6...v0.1.0-rc.0) (2026-06-16)


### ⚠ BREAKING CHANGES

* refresh current room on reconnect ([#878](https://github.com/chattocorp/chatto/issues/878))
* **auth:** stabilize cookie session auth ([#883](https://github.com/chattocorp/chatto/issues/883))
* simplify RBAC permissions ([#880](https://github.com/chattocorp/chatto/issues/880))

### Features

* add per-process Prometheus metrics ([#877](https://github.com/chattocorp/chatto/issues/877)) ([34a88e5](https://github.com/chattocorp/chatto/commit/34a88e5b3608f87b778ecbc3a67120df404cbb30))
* **auth:** support external auth providers ([#873](https://github.com/chattocorp/chatto/issues/873)) ([ff2fb06](https://github.com/chattocorp/chatto/commit/ff2fb0681832cd1915004117b27b0cc43781a782))
* make LiveKit reconciliation resilient ([#869](https://github.com/chattocorp/chatto/issues/869)) ([82a5bc9](https://github.com/chattocorp/chatto/commit/82a5bc937c503203ae2bc557cc788f1a14c47b0b))
* show call lifecycle notices in room events ([#867](https://github.com/chattocorp/chatto/issues/867)) ([b652c4f](https://github.com/chattocorp/chatto/commit/b652c4f9511359bc89b68ccf51ec4a232317ea5d))


### Bug Fixes

* **auth:** stabilize cookie session auth ([#883](https://github.com/chattocorp/chatto/issues/883)) ([376a268](https://github.com/chattocorp/chatto/commit/376a268595420601f78c328fae38969648638644))
* **cli:** improve generated chatto config defaults ([#872](https://github.com/chattocorp/chatto/issues/872)) ([7ba64b7](https://github.com/chattocorp/chatto/commit/7ba64b779dbdd8ee4147dcc541ea19d1960a213e))
* **config:** tighten chatto config validation ([#868](https://github.com/chattocorp/chatto/issues/868)) ([8b45012](https://github.com/chattocorp/chatto/commit/8b450122fd52e043fecea4cb87042ae2ba73df1a))
* **core:** align projection snapshots with OCC ([#864](https://github.com/chattocorp/chatto/issues/864)) ([f805493](https://github.com/chattocorp/chatto/commit/f80549386bcab39a0cb2a2874cd0724b7dac8fc9))
* **frontend:** prevent expired edit via ArrowUp ([#879](https://github.com/chattocorp/chatto/issues/879)) ([bbae3aa](https://github.com/chattocorp/chatto/commit/bbae3aa576a7a036f7567753bb38925afbd1bea6))
* ignore markdown code mentions and previews ([#866](https://github.com/chattocorp/chatto/issues/866)) ([37933cb](https://github.com/chattocorp/chatto/commit/37933cbd552e406ee7e2ad5a48d7f56449886ce5))
* refresh current room on reconnect ([#878](https://github.com/chattocorp/chatto/issues/878)) ([8066af7](https://github.com/chattocorp/chatto/commit/8066af79bc669ad613a496615719a103385c70d2))
* remember sidebar visibility preferences ([#862](https://github.com/chattocorp/chatto/issues/862)) ([ec13041](https://github.com/chattocorp/chatto/commit/ec130411d1a6279e3e5ad218f77281d2382d7e55))


### Code Refactoring

* simplify RBAC permissions ([#880](https://github.com/chattocorp/chatto/issues/880)) ([37fe2c6](https://github.com/chattocorp/chatto/commit/37fe2c6dac274a4edf48c5051b7ecfcb04dcdcfb))

## [0.1.0-beta.6](https://github.com/chattocorp/chatto/compare/v0.1.0-beta.5...v0.1.0-beta.6) (2026-06-15)


### Features

* add durable LiveKit call events and E2EE ([#835](https://github.com/chattocorp/chatto/issues/835)) ([8d91797](https://github.com/chattocorp/chatto/commit/8d91797e842e68072f14fcd2aa9543c2ade1d477))
* add role mentions ([#825](https://github.com/chattocorp/chatto/issues/825)) ([cc95f73](https://github.com/chattocorp/chatto/commit/cc95f73460e868cd41cb6103f8b6587c79d38010))
* add room extras sidebar tabs ([#856](https://github.com/chattocorp/chatto/issues/856)) ([99dff21](https://github.com/chattocorp/chatto/commit/99dff210ddb95b7c4162d1f63767f4e951f6ff4a))
* **admin:** auto-paginate event log ([#852](https://github.com/chattocorp/chatto/issues/852)) ([cbee54f](https://github.com/chattocorp/chatto/commit/cbee54fa88bf6e47424a30e9f92ef7b16b05da66))
* allow editing thread reply channel echoes ([#847](https://github.com/chattocorp/chatto/issues/847)) ([a5abd5a](https://github.com/chattocorp/chatto/commit/a5abd5a3b4b2c1c06504fcdbd5a512c8346405d6))
* **frontend:** find server users in cmd-k ([#844](https://github.com/chattocorp/chatto/issues/844)) ([26283ce](https://github.com/chattocorp/chatto/commit/26283ce5818766fa4a94bc147f6a865478669d68))


### Bug Fixes

* add CSRF protection for cookie sessions ([#851](https://github.com/chattocorp/chatto/issues/851)) ([ccc8d69](https://github.com/chattocorp/chatto/commit/ccc8d6961d8e05095b025d8ea89101d604258e9d))
* attribute RBAC audit events to actors ([#834](https://github.com/chattocorp/chatto/issues/834)) ([0e89890](https://github.com/chattocorp/chatto/commit/0e898907f45da420c6728e75ff4b7fe86ae34911))
* **core:** end stuck calls when LiveKit fails ([#860](https://github.com/chattocorp/chatto/issues/860)) ([fbe1644](https://github.com/chattocorp/chatto/commit/fbe1644f931b8cadb3a2ed457557450fc89adb09))
* **frontend:** auto-paginate admin members ([#846](https://github.com/chattocorp/chatto/issues/846)) ([7fff051](https://github.com/chattocorp/chatto/commit/7fff0510133d31d31ed412ef639ab374e03970bd))
* **frontend:** paginate room member sidebar ([#833](https://github.com/chattocorp/chatto/issues/833)) ([1e87d98](https://github.com/chattocorp/chatto/commit/1e87d9855e9c2918539085a76780a6c5d19df226))
* **frontend:** remove server header leave icon ([#855](https://github.com/chattocorp/chatto/issues/855)) ([360bdca](https://github.com/chattocorp/chatto/commit/360bdcabd458eb7d0f8b16bac649b8c940c1b217))
* **frontend:** stabilize presence display ([#850](https://github.com/chattocorp/chatto/issues/850)) ([1901ca2](https://github.com/chattocorp/chatto/commit/1901ca24982a879b242001951ccd0e2080ee8198))
* **frontend:** use commit hash for dev app version ([#857](https://github.com/chattocorp/chatto/issues/857)) ([2a7f73e](https://github.com/chattocorp/chatto/commit/2a7f73ee3eb2b594db916a29d6c93cf2ad73b450))
* **logging:** stop logging user PII ([#830](https://github.com/chattocorp/chatto/issues/830)) ([6f1b558](https://github.com/chattocorp/chatto/commit/6f1b558278f2216e88ab02a93df59579fbec2be8))
* preserve session auth for GraphQL CSRF ([#858](https://github.com/chattocorp/chatto/issues/858)) ([4b1507d](https://github.com/chattocorp/chatto/commit/4b1507d7826e89bb967adec16f1e12ded14534fa))
* refine conversation start marker UX ([#839](https://github.com/chattocorp/chatto/issues/839)) ([862a617](https://github.com/chattocorp/chatto/commit/862a617b216fe3cf4dab7099163ca36a6696de87))
* replay missed subscription events ([#832](https://github.com/chattocorp/chatto/issues/832)) ([eeec111](https://github.com/chattocorp/chatto/commit/eeec111e41fc6037d53e22a932f9e8a209b80440))
* validate cookie encryption secret early ([#842](https://github.com/chattocorp/chatto/issues/842)) ([899953c](https://github.com/chattocorp/chatto/commit/899953ce48b277e4488fd0f01e0d316033ddc16c))


### Performance Improvements

* **threads:** paginate My Threads ([#837](https://github.com/chattocorp/chatto/issues/837)) ([7d4afab](https://github.com/chattocorp/chatto/commit/7d4afab47f0054b756c290a8a8c72fd752589b93))

## [0.1.0-beta.5](https://github.com/chattocorp/chatto/compare/v0.1.0-beta.4...v0.1.0-beta.5) (2026-06-13)


### Bug Fixes

* **frontend:** cache reply previews during scroll ([#819](https://github.com/chattocorp/chatto/issues/819)) ([fc2c629](https://github.com/chattocorp/chatto/commit/fc2c62963909c692a91c36151958b3aceb959de5))
* **frontend:** crop server sidebar banners ([#822](https://github.com/chattocorp/chatto/issues/822)) ([41ad36b](https://github.com/chattocorp/chatto/commit/41ad36b1756dca529eaba8a255f0f3789533f6d1))
* ignore foreign LiveKit webhooks ([de90c89](https://github.com/chattocorp/chatto/commit/de90c89a4356634eaf956ee14ad650bbb3aedd9a))

## [0.1.0-beta.4](https://github.com/chattocorp/chatto/compare/v0.1.0-beta.3...v0.1.0-beta.4) (2026-06-12)


### Features

* **pwa:** enrich web app manifest ([#808](https://github.com/chattocorp/chatto/issues/808)) ([2c6fe8b](https://github.com/chattocorp/chatto/commit/2c6fe8be747f7041706128c43c5d97403ca8a4cf))


### Bug Fixes

* emit structured logs for Loki ([#815](https://github.com/chattocorp/chatto/issues/815)) ([25ab64a](https://github.com/chattocorp/chatto/commit/25ab64a48d4bea686bf2c2e09a11d0f5e711f562))
* harden backend shutdown handling ([#814](https://github.com/chattocorp/chatto/issues/814)) ([59d344b](https://github.com/chattocorp/chatto/commit/59d344b5839c252e12ab88b74d5fc9d16bece5f6))
* Harden Docker images ([0b227e9](https://github.com/chattocorp/chatto/commit/0b227e9c131ddab9983b3fa07d152ca80cfb441e))
* improve web push provider compatibility ([#816](https://github.com/chattocorp/chatto/issues/816)) ([2e0d464](https://github.com/chattocorp/chatto/commit/2e0d464b141c821c673b74cea2235265617943c2))
* **projections:** fail visibly on projection errors ([#803](https://github.com/chattocorp/chatto/issues/803)) ([6959161](https://github.com/chattocorp/chatto/commit/695916195f1a3aaa087b5264f2cec95f8fa12070))
* **projections:** introduce stream positions and services ([#812](https://github.com/chattocorp/chatto/issues/812)) ([240970c](https://github.com/chattocorp/chatto/commit/240970c749cf4da90fad6a23b163b3a96550d465))

## [0.1.0-beta.3](https://github.com/chattocorp/chatto/compare/v0.1.0-beta.2...v0.1.0-beta.3) (2026-06-12)


### Bug Fixes

* **timeline:** preserve migrated room join order ([#801](https://github.com/chattocorp/chatto/issues/801)) ([53547ca](https://github.com/chattocorp/chatto/commit/53547ca794af634fe60bcbcaa98fc7477bb64da1))

## [0.1.0-beta.2](https://github.com/chattocorp/chatto/compare/v0.1.0-beta.1...v0.1.0-beta.2) (2026-06-11)


### Features

* **proto:** stabilize event schemas for beta ([#797](https://github.com/chattocorp/chatto/issues/797)) ([ef3c601](https://github.com/chattocorp/chatto/commit/ef3c6018b4d112c00e320d301e0c6b94156cb53b))

## [0.1.0-beta.1](https://github.com/chattocorp/chatto/compare/v0.1.0-beta.0...v0.1.0-beta.1) (2026-06-11)


### Bug Fixes

* **auth:** add OAuth redirect origin allowlist ([#796](https://github.com/chattocorp/chatto/issues/796)) ([7cbc486](https://github.com/chattocorp/chatto/commit/7cbc486b371bedde2cdb0e9d59d09259f2fa0b90))
* **auth:** include server name in auth emails ([#793](https://github.com/chattocorp/chatto/issues/793)) ([19dd784](https://github.com/chattocorp/chatto/commit/19dd78470adac1e773fe91440c8ea354a06224e0))

## [0.1.0-beta.0](https://github.com/chattocorp/chatto/compare/v0.1.0-alpha.3...v0.1.0-beta.0) (2026-06-10)


### Features

* add s3 asset path prefix ([#784](https://github.com/chattocorp/chatto/issues/784)) ([bbf0262](https://github.com/chattocorp/chatto/commit/bbf02628114a44decab802285b3f9559f0a5597e))
* **auth:** add OAuth consent flow ([#791](https://github.com/chattocorp/chatto/issues/791)) ([b401b57](https://github.com/chattocorp/chatto/commit/b401b57ac8d95b7cbba14d4b7650b4adb31ba8d7))
* **frontend:** inline admin sidebar navigation ([#785](https://github.com/chattocorp/chatto/issues/785)) ([0be5f68](https://github.com/chattocorp/chatto/commit/0be5f6887be92797730fb8a6b48aa36fcf19529d))
* **moderation:** add channel room bans ([#777](https://github.com/chattocorp/chatto/issues/777)) ([abc107b](https://github.com/chattocorp/chatto/commit/abc107b0fd188be62e5d676d0b81d2a3596d5a6c))
* proxy asset URLs through service worker ([#781](https://github.com/chattocorp/chatto/issues/781)) ([309d0b0](https://github.com/chattocorp/chatto/commit/309d0b09be68e127d94c4e7da5d46d9f91e0a993))


### Bug Fixes

* **assets:** sandbox active attachment responses ([#788](https://github.com/chattocorp/chatto/issues/788)) ([f98f826](https://github.com/chattocorp/chatto/commit/f98f82694441dd359983b9ad078a4ae20d5bd1dd))
* **auth:** restrict OAuth redirect origins ([#786](https://github.com/chattocorp/chatto/issues/786)) ([50268a6](https://github.com/chattocorp/chatto/commit/50268a6e41188c920c729300253eaf83375cd79a))
* consolidate server config live events ([#783](https://github.com/chattocorp/chatto/issues/783)) ([995e663](https://github.com/chattocorp/chatto/commit/995e663b96ffada126a21e0b5256830ad296fe93))
* **es:** canonicalize legacy import verification ([1af33ac](https://github.com/chattocorp/chatto/commit/1af33ac34ca03fad9c05951b9a23cd81fa63e986))
* refresh expiring attachment asset URLs ([#779](https://github.com/chattocorp/chatto/issues/779)) ([2de2dde](https://github.com/chattocorp/chatto/commit/2de2ddeda62e8493ae59f409bd82434711dbca08))


### Miscellaneous Chores

* force beta prerelease ([c6833b4](https://github.com/chattocorp/chatto/commit/c6833b41b15c9a4ccd7d772ead3684d641134ae1))

## [0.1.0-alpha.3](https://github.com/chattocorp/chatto/compare/v0.1.0-alpha.2...v0.1.0-alpha.3) (2026-06-08)


### ⚠ BREAKING CHANGES

* **graphql:** consolidate list field shapes ([#770](https://github.com/chattocorp/chatto/issues/770))

### Features

* add compact encrypted data envelopes ([#704](https://github.com/chattocorp/chatto/issues/704)) ([4c6b7b6](https://github.com/chattocorp/chatto/commit/4c6b7b644f57b12a4c92b161caa7a331286c9d57))
* add ES rollout observability ([#709](https://github.com/chattocorp/chatto/issues/709)) ([2c0cb34](https://github.com/chattocorp/chatto/commit/2c0cb348589fd7234cf7424e2f8b4dfe7bf2e789))
* add explicit room thread creation events ([#722](https://github.com/chattocorp/chatto/issues/722)) ([2de3459](https://github.com/chattocorp/chatto/commit/2de345947400916514ad40759f3719242fa87489))
* add server-admin system diagnostics ([#720](https://github.com/chattocorp/chatto/issues/720)) ([64e23f0](https://github.com/chattocorp/chatto/commit/64e23f0719905037feaaf1073a2e5a93548997df))
* add server-side cookie sessions ([#732](https://github.com/chattocorp/chatto/issues/732)) ([3a0b224](https://github.com/chattocorp/chatto/commit/3a0b224507a99cf2b5c6f355f9362a59cc4d4ae8))
* add shreddable message body events ([#729](https://github.com/chattocorp/chatto/issues/729)) ([ea05797](https://github.com/chattocorp/chatto/commit/ea057972b3f96e5a73d70441de420d8413415c85))
* audit auth token workflows ([#697](https://github.com/chattocorp/chatto/issues/697)) ([fce12a4](https://github.com/chattocorp/chatto/commit/fce12a42c49944777e81a3816db87ccdaf677d86))
* **auth:** use OTP codes for email verification ([#771](https://github.com/chattocorp/chatto/issues/771)) ([0bf1905](https://github.com/chattocorp/chatto/commit/0bf19057102cc16eb1baa43f45b17f0183233d77))
* **frontend:** polish service worker shell caching ([#773](https://github.com/chattocorp/chatto/issues/773)) ([b842901](https://github.com/chattocorp/chatto/commit/b842901ed23ba2ec1af243fb28a456facbd776be))
* **graphql:** clean up schema hygiene ([#724](https://github.com/chattocorp/chatto/issues/724)) ([f68ae54](https://github.com/chattocorp/chatto/commit/f68ae54eb3786aa8c9eb3bac6577bc2597d3bade))
* harden encryption key storage ([#710](https://github.com/chattocorp/chatto/issues/710)) ([0bf76e7](https://github.com/chattocorp/chatto/commit/0bf76e7d1199cd89853344ee73ea6402393a7a72))
* move presence and calls to memory cache ([#702](https://github.com/chattocorp/chatto/issues/702)) ([c98aacf](https://github.com/chattocorp/chatto/commit/c98aacf52fb4c1dd444270e3b547443ed841d6c5))
* store link preview cache in runtime state ([#708](https://github.com/chattocorp/chatto/issues/708)) ([d5832c4](https://github.com/chattocorp/chatto/commit/d5832c41ce92de5ee9125547eb1c0eb74ae78fd6))


### Bug Fixes

* add GraphQL length validation ([#751](https://github.com/chattocorp/chatto/issues/751)) ([715a3b4](https://github.com/chattocorp/chatto/commit/715a3b4635ba4f1cacf40d1a19f5346c9ab30d5a))
* add HTTP server timeout hardening ([#723](https://github.com/chattocorp/chatto/issues/723)) ([880628e](https://github.com/chattocorp/chatto/commit/880628e98e8a4e322e08f88124257b72fcf59d9f))
* add report-only CSP header ([#728](https://github.com/chattocorp/chatto/issues/728)) ([74e6200](https://github.com/chattocorp/chatto/commit/74e62006b575e75836ff833d35e7b93aca56f9d5))
* **auth:** revoke credentials after password changes ([#752](https://github.com/chattocorp/chatto/issues/752)) ([e1adcbd](https://github.com/chattocorp/chatto/commit/e1adcbd4a23110e6f1b9808a5fea9f467d42bd7f))
* autofocus login identifier field ([#727](https://github.com/chattocorp/chatto/issues/727)) ([f349bba](https://github.com/chattocorp/chatto/commit/f349bba0c5dd903f22efc8b54d1989b889380585))
* clamp room event query limits ([#735](https://github.com/chattocorp/chatto/issues/735)) ([75bf8e0](https://github.com/chattocorp/chatto/commit/75bf8e064c08a6006570990cae87af150486e60d))
* clean up cached asset derivatives on deletion ([#766](https://github.com/chattocorp/chatto/issues/766)) ([f7a6d04](https://github.com/chattocorp/chatto/commit/f7a6d04517e72281f1d3f9241631cba0ed077700))
* **core:** consolidate NATS asset storage ([#768](https://github.com/chattocorp/chatto/issues/768)) ([1eaca2b](https://github.com/chattocorp/chatto/commit/1eaca2b93492d17b674af1e9c69e34751c4f6919))
* disable video uploads when processing is off ([#695](https://github.com/chattocorp/chatto/issues/695)) ([4a31d1a](https://github.com/chattocorp/chatto/commit/4a31d1a1d07d948bc933d73fb9194c6bdd1aa7f3))
* enforce core string length limits ([#741](https://github.com/chattocorp/chatto/issues/741)) ([3c64b17](https://github.com/chattocorp/chatto/commit/3c64b17af6d723fb8c3597a4d84e970babf347a2))
* **frontend:** disable composer submit while attachments stage ([#711](https://github.com/chattocorp/chatto/issues/711)) ([fdb1831](https://github.com/chattocorp/chatto/commit/fdb1831b5b5fabb402a4c021ceb39aca73ae0f70))
* **frontend:** keep failed server icons visible ([#772](https://github.com/chattocorp/chatto/issues/772)) ([7b974d6](https://github.com/chattocorp/chatto/commit/7b974d6a4e52f01c8735ce8b311f91af6d486ddc))
* **graphql:** widen event log total count ([#760](https://github.com/chattocorp/chatto/issues/760)) ([79ebf41](https://github.com/chattocorp/chatto/commit/79ebf414332077a6bfc96df23202c6902c7de645))
* harden OIDC avatar fetching ([#739](https://github.com/chattocorp/chatto/issues/739)) ([7b82ad7](https://github.com/chattocorp/chatto/commit/7b82ad7a997533a0d1959e2f52fc060bb606a88d))
* hide echoes on direct retraction ([#701](https://github.com/chattocorp/chatto/issues/701)) ([035601b](https://github.com/chattocorp/chatto/commit/035601bdedceae0255ca07ccd6e5cf689a1ec4f2))
* limit GraphQL JSON request body size ([#740](https://github.com/chattocorp/chatto/issues/740)) ([8cae516](https://github.com/chattocorp/chatto/commit/8cae5164f15a0adf98d95746b5cf01fffea4a2c3))
* make message ES importer non-atomic ([#733](https://github.com/chattocorp/chatto/issues/733)) ([651780b](https://github.com/chattocorp/chatto/commit/651780bb0d3f0ccdd80f009f6319467bb77fcc70))
* paginate unbounded GraphQL list fields ([#726](https://github.com/chattocorp/chatto/issues/726)) ([1e7d5e8](https://github.com/chattocorp/chatto/commit/1e7d5e802e509447584b2c83ce60c100065e5ebb))
* require mandatory SMTP TLS by default ([#725](https://github.com/chattocorp/chatto/issues/725)) ([ecad9c5](https://github.com/chattocorp/chatto/commit/ecad9c5c6fbe6a4b036c902643740c306a245183))


### Performance Improvements

* optimize room timeline projection reads ([#734](https://github.com/chattocorp/chatto/issues/734)) ([2265ee8](https://github.com/chattocorp/chatto/commit/2265ee8e7c2dc845ee857b2cb714c4cebba80ca7))


### Code Refactoring

* **graphql:** consolidate list field shapes ([#770](https://github.com/chattocorp/chatto/issues/770)) ([b20beda](https://github.com/chattocorp/chatto/commit/b20beda1ee92395f1dddde831c7a44dcc3679203))

## [0.1.0-alpha.2](https://github.com/chattocorp/chatto/compare/v0.1.0-alpha.1...v0.1.0-alpha.2) (2026-06-01)


### Features

* add EVT auth audit events ([#687](https://github.com/chattocorp/chatto/issues/687)) ([dc50aa2](https://github.com/chattocorp/chatto/commit/dc50aa2d126f3891b5a490a27d8eace297db8bcc))
* hmac runtime token storage ([#688](https://github.com/chattocorp/chatto/issues/688)) ([c9d0065](https://github.com/chattocorp/chatto/commit/c9d0065d809da2db45972b2b2096ff7f53ee710c))
* remove DM-specific permissions ([#683](https://github.com/chattocorp/chatto/issues/683)) ([5efe07b](https://github.com/chattocorp/chatto/commit/5efe07b0e8733bc98000100b1d893eabc9982600))


### Bug Fixes

* **frontend:** disable composer submit while attachments stage ([#711](https://github.com/chattocorp/chatto/issues/711)) ([fdb1831](https://github.com/chattocorp/chatto/commit/fdb1831b5b5fabb402a4c021ceb39aca73ae0f70))
* move thread follow state to runtime state ([#685](https://github.com/chattocorp/chatto/issues/685)) ([bb052ba](https://github.com/chattocorp/chatto/commit/bb052ba787a4c5963854aa4945269ce08f5f7296))
* stabilize scroll fade overlays ([#681](https://github.com/chattocorp/chatto/issues/681)) ([d471189](https://github.com/chattocorp/chatto/commit/d471189f24802b9024f25883acb8ccfed8fe7e63))

## [0.1.0-alpha.1](https://github.com/chattocorp/chatto/compare/v0.1.0-alpha.0...v0.1.0-alpha.1) (2026-05-30)


### Bug Fixes

* apply config owners on startup ([#679](https://github.com/chattocorp/chatto/issues/679)) ([e695255](https://github.com/chattocorp/chatto/commit/e695255faca58ee8ebb177564d05ce61ad20e4c6))
* **ci:** let next prereleases increment ([4a14557](https://github.com/chattocorp/chatto/commit/4a14557472746fc18a8b5365bf45adbb2f70265f))
* **ci:** use prerelease versioning on next ([833a8a1](https://github.com/chattocorp/chatto/commit/833a8a1bc7482244a403c22b365087d030a2c5aa))
* deduplicate room join events ([#672](https://github.com/chattocorp/chatto/issues/672)) ([a018184](https://github.com/chattocorp/chatto/commit/a0181849bed524565a33a9fde72276e14486cfa6))

## [0.1.0-alpha.0](https://github.com/chattocorp/chatto/compare/v0.0.189...v0.1.0-alpha.0) (2026-05-29)


### Features

* **admin:** add projection runtime diagnostics ([#646](https://github.com/chattocorp/chatto/issues/646)) ([178cd8e](https://github.com/chattocorp/chatto/commit/178cd8e884dea7f8f5808527947b07d3ac2ed562))
* **core:** messages and threads projections for event-sourced reads ([#614](https://github.com/chattocorp/chatto/issues/614)) ([a8b5585](https://github.com/chattocorp/chatto/commit/a8b55856937d3985f9c39af8151986bc52e2c0fc))
* **es:** harden local rollout imports ([#642](https://github.com/chattocorp/chatto/issues/642)) ([82207b2](https://github.com/chattocorp/chatto/commit/82207b22dae0bc25a953b7cc5060994992cc7465))
* event-source user accounts ([#650](https://github.com/chattocorp/chatto/issues/650)) ([7964a63](https://github.com/chattocorp/chatto/commit/7964a63d2d8be993f465f248e95f924822e78a1e))
* **graphql:** expose message edit events ([#664](https://github.com/chattocorp/chatto/issues/664)) ([f31c62a](https://github.com/chattocorp/chatto/commit/f31c62ad45e7d4c7ff72faa40200fc419d76e387))
* move video asset manifests into EVT ([#669](https://github.com/chattocorp/chatto/issues/669)) ([0e75502](https://github.com/chattocorp/chatto/commit/0e75502827ae60b471d407251aeaf8a1f9ca7d41))
* **proto:** durable message edit/retract events for ES migration ([#606](https://github.com/chattocorp/chatto/issues/606)) ([c237a46](https://github.com/chattocorp/chatto/commit/c237a46d7b91b6fc4369eec8754b34cab7d97f07))
* **reactions:** move reactions to event sourcing ([#635](https://github.com/chattocorp/chatto/issues/635)) ([e8140b6](https://github.com/chattocorp/chatto/commit/e8140b65358adc515f46db87255c0a44b84f8dd2))
* **storage:** move read markers to runtime state ([#661](https://github.com/chattocorp/chatto/issues/661)) ([14131d3](https://github.com/chattocorp/chatto/commit/14131d3de48696fb4558c7de3031b2b4f31d3ae6))


### Bug Fixes

* **ci:** start the prerelease line on 0.1.0-alpha.0 ([#613](https://github.com/chattocorp/chatto/issues/613)) ([6a4b767](https://github.com/chattocorp/chatto/commit/6a4b7671191edb676d55657090a9647842272676))
* **ci:** stop release-please runaway PR loop ([#622](https://github.com/chattocorp/chatto/issues/622)) ([49e6350](https://github.com/chattocorp/chatto/commit/49e6350e30403743122d880ec44366eb01bfc803))
* **ci:** tighten release-please trigger to not match its own branches ([03dea0f](https://github.com/chattocorp/chatto/commit/03dea0f27f3ac3119646dfe1eb286513f0b72859))
* **es:** harden event-sourcing OCC behavior ([#649](https://github.com/chattocorp/chatto/issues/649)) ([8dd6783](https://github.com/chattocorp/chatto/commit/8dd67831c84a319fcb9883975ffe441bef1879f1))
* **es:** preserve imported thread replies ([#648](https://github.com/chattocorp/chatto/issues/648)) ([d64a045](https://github.com/chattocorp/chatto/commit/d64a045ccc146b3dc97489d0ebf02813ce010ce6))
* **frontend:** catch up missed messages after sleep + refactor message-store lifecycle ([#631](https://github.com/chattocorp/chatto/issues/631)) ([1bf2c51](https://github.com/chattocorp/chatto/commit/1bf2c51598d6df109558aa90013addb1ebfb77ca))
* **frontend:** clean utility story links ([#653](https://github.com/chattocorp/chatto/issues/653)) ([06e608f](https://github.com/chattocorp/chatto/commit/06e608f96c4f0a8d2ac155144d8f3581d5592c41))
* **frontend:** refresh attachment URLs on lightbox open and download click ([#616](https://github.com/chattocorp/chatto/issues/616)) ([23973ac](https://github.com/chattocorp/chatto/commit/23973acb977e1cfa8b8149885c0ba23ce1e7a315))
* **frontend:** refresh scroll fades on content changes ([1f01dbe](https://github.com/chattocorp/chatto/commit/1f01dbe4da2449300bed9ee2229da38b4f6db1f3))
* refresh attachment URLs for image viewer ([#637](https://github.com/chattocorp/chatto/issues/637)) ([1324ce1](https://github.com/chattocorp/chatto/commit/1324ce1970d3d5077eae5bcadd002adcbae6f247))

## [0.0.192](https://github.com/chattocorp/chatto/compare/v0.0.191...v0.0.192) (2026-05-26)


### Bug Fixes

* **frontend:** refresh scroll fades on content changes ([1f01dbe](https://github.com/chattocorp/chatto/commit/1f01dbe4da2449300bed9ee2229da38b4f6db1f3))
* refresh attachment URLs for image viewer ([#637](https://github.com/chattocorp/chatto/issues/637)) ([1324ce1](https://github.com/chattocorp/chatto/commit/1324ce1970d3d5077eae5bcadd002adcbae6f247))

## [0.0.191](https://github.com/chattocorp/chatto/compare/v0.0.190...v0.0.191) (2026-05-26)


### Bug Fixes

* **frontend:** catch up missed messages after sleep + refactor message-store lifecycle ([#631](https://github.com/chattocorp/chatto/issues/631)) ([1bf2c51](https://github.com/chattocorp/chatto/commit/1bf2c51598d6df109558aa90013addb1ebfb77ca))

## [0.0.190](https://github.com/chattocorp/chatto/compare/v0.0.189...v0.0.190) (2026-05-25)


### Bug Fixes

* **ci:** stop release-please runaway PR loop ([#622](https://github.com/chattocorp/chatto/issues/622)) ([49e6350](https://github.com/chattocorp/chatto/commit/49e6350e30403743122d880ec44366eb01bfc803))
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
