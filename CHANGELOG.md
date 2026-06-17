# Changelog

All notable changes to Chatto. Maintained by release-please from the
conventional-commit messages on `main` — do not edit by hand.

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
