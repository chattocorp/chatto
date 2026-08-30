# Changelog

All notable changes to Chatto. Maintained by release-please from the
conventional-commit messages on `main` — do not edit by hand.

## [0.5.0-alpha.4](https://github.com/chattocorp/chatto/compare/v0.5.0-alpha.3...v0.5.0-alpha.4) (2026-08-30)


### ⚠ BREAKING CHANGES

* **bots:** support multiple named API keys ([#2244](https://github.com/chattocorp/chatto/issues/2244))

### Features

* **bots:** support multiple named API keys ([#2244](https://github.com/chattocorp/chatto/issues/2244)) ([1ca3a9e](https://github.com/chattocorp/chatto/commit/1ca3a9e33ce360b4be5611bb4ebfe07362812f51))
* **frontend:** add bounded recursive neighbor discovery ([#2230](https://github.com/chattocorp/chatto/issues/2230)) ([46c3045](https://github.com/chattocorp/chatto/commit/46c304521868d83c662c2f9e7944f660127a2fe5))
* **frontend:** add user menu action icons ([#2222](https://github.com/chattocorp/chatto/issues/2222)) ([98a4a4c](https://github.com/chattocorp/chatto/commit/98a4a4c0a90fa9ebb65f2a650fe11c41e623f867))
* **frontend:** finish the progressive Neighbor tapestry ([#2234](https://github.com/chattocorp/chatto/issues/2234)) ([ba864c5](https://github.com/chattocorp/chatto/commit/ba864c566e33800c6e23a58e89daaf57a39ee677))
* **frontend:** simplify push notification activation ([#2228](https://github.com/chattocorp/chatto/issues/2228)) ([482fcf3](https://github.com/chattocorp/chatto/commit/482fcf3b971d9905e597226bfd84b7181db66a1b))
* manage rooms from the server sidebar ([#2219](https://github.com/chattocorp/chatto/issues/2219)) ([eb81244](https://github.com/chattocorp/chatto/commit/eb812440d812b884f73a4b164e708edc4b9f7f52))
* **mcp:** add authenticated network integration ([#2225](https://github.com/chattocorp/chatto/issues/2225)) ([45eaece](https://github.com/chattocorp/chatto/commit/45eaece641aff4921152fc3606fdbbba772f9683))
* **neighbors:** add public testimonials ([#2223](https://github.com/chattocorp/chatto/issues/2223)) ([a9f611c](https://github.com/chattocorp/chatto/commit/a9f611c200016d3dfc4b501f75c2c0816b32f0ff))
* **neighbors:** show recommendation sources ([#2202](https://github.com/chattocorp/chatto/issues/2202)) ([4c2a9fb](https://github.com/chattocorp/chatto/commit/4c2a9fb612931df7f02ee2ad7603bc37e76855da))


### Bug Fixes

* **ci:** update only affected release PRs ([#2203](https://github.com/chattocorp/chatto/issues/2203)) ([1ea5e54](https://github.com/chattocorp/chatto/commit/1ea5e54e867b3c0b1645a00f454b979b87357e59))
* **frontend:** layer jump button above scroll fade ([#2209](https://github.com/chattocorp/chatto/issues/2209)) ([5ebd2d6](https://github.com/chattocorp/chatto/commit/5ebd2d6570975906d608edd76e2bb3ac71169db5))
* **frontend:** make notification dismissal safer ([#2204](https://github.com/chattocorp/chatto/issues/2204)) ([19d99ba](https://github.com/chattocorp/chatto/commit/19d99bab4d3f86e4fd7000f31413677b9170d4bd))
* **frontend:** recover unread markers after app resume ([#2229](https://github.com/chattocorp/chatto/issues/2229)) ([36fa5ea](https://github.com/chattocorp/chatto/commit/36fa5ea9c15f236424f765134d574ef3f4b8ec67))
* **frontend:** standardize settings form actions ([#2201](https://github.com/chattocorp/chatto/issues/2201)) ([e2ce334](https://github.com/chattocorp/chatto/commit/e2ce3340cae51252fa8592221f55680e161a4101))
* **neighbors:** hand incompatible servers off to their client ([#2227](https://github.com/chattocorp/chatto/issues/2227)) ([4b951ea](https://github.com/chattocorp/chatto/commit/4b951eadab952c163f6bbfea52a3c46ec9a5daf9))
* **neighbors:** reject server origins ([#2210](https://github.com/chattocorp/chatto/issues/2210)) ([69db917](https://github.com/chattocorp/chatto/commit/69db917847e31269be85ff996c099a471eda93f7))

## [0.5.0-alpha.3](https://github.com/chattocorp/chatto/compare/v0.5.0-alpha.2...v0.5.0-alpha.3) (2026-08-29)


### Features

* **frontend:** finish version easter egg ([#2192](https://github.com/chattocorp/chatto/issues/2192)) ([bef8139](https://github.com/chattocorp/chatto/commit/bef8139665d3c94d738a33283c7f6ffabb0ad4e9))
* **frontend:** restore configurable thread overlays ([#2191](https://github.com/chattocorp/chatto/issues/2191)) ([9e89dd3](https://github.com/chattocorp/chatto/commit/9e89dd30b93956018fad7958eba6f043356bb234))
* **frontend:** unify checkbox and choice row styling ([#2199](https://github.com/chattocorp/chatto/issues/2199)) ([c3b3908](https://github.com/chattocorp/chatto/commit/c3b390815c0fc5f8c6b73f0eba6952e251b5b6af))
* **neighbors:** add advertised server directory ([#2183](https://github.com/chattocorp/chatto/issues/2183)) ([eebf6df](https://github.com/chattocorp/chatto/commit/eebf6dfba808538ab42df3d0eba8fa3f6905262d))
* **privacy:** make timezone sharing opt-in ([#2195](https://github.com/chattocorp/chatto/issues/2195)) ([f660d21](https://github.com/chattocorp/chatto/commit/f660d216a83ed1b67ac4711614e8006d8956ae6a))


### Bug Fixes

* **auth:** extend email verification codes to 30 minutes ([#2197](https://github.com/chattocorp/chatto/issues/2197)) ([11e16b3](https://github.com/chattocorp/chatto/commit/11e16b3011f803c124e8e8567e59812531a91231))
* **ci:** roll Go build cache between commits ([#2188](https://github.com/chattocorp/chatto/issues/2188)) ([4685b4e](https://github.com/chattocorp/chatto/commit/4685b4efe98642a582d1fdd2edbd67f5bf9b0b6d))
* **frontend:** keep current room text regular ([#2194](https://github.com/chattocorp/chatto/issues/2194)) ([eb44b8c](https://github.com/chattocorp/chatto/commit/eb44b8c720af0a989ee414fc78a08a2824ee6713))
* **frontend:** open settings on appearance ([#2193](https://github.com/chattocorp/chatto/issues/2193)) ([32317e5](https://github.com/chattocorp/chatto/commit/32317e599944ed2bbe9f729f42cf3f55adae4d3b))
* **frontend:** restore bright notification orange ([#2189](https://github.com/chattocorp/chatto/issues/2189)) ([bede63d](https://github.com/chattocorp/chatto/commit/bede63d734ab0d6e5e3e5b920608790166bb28d0))
* **frontend:** restore visual composer caret ([#2196](https://github.com/chattocorp/chatto/issues/2196)) ([3d29dc5](https://github.com/chattocorp/chatto/commit/3d29dc51d1b8285f3411e6078049e6d978edb21a))
* **frontend:** stop automatic app update reloads ([#2198](https://github.com/chattocorp/chatto/issues/2198)) ([6caada1](https://github.com/chattocorp/chatto/commit/6caada12479f517f45f10b9220a3acd4c0bf27dc))

## [0.5.0-alpha.2](https://github.com/chattocorp/chatto/compare/v0.5.0-alpha.1...v0.5.0-alpha.2) (2026-08-29)


### Features

* **frontend:** add compact composer formatting shelf ([#2182](https://github.com/chattocorp/chatto/issues/2182)) ([98c072b](https://github.com/chattocorp/chatto/commit/98c072bdec558975759086e3a2b3b594980b5356))


### Bug Fixes

* **auth:** publish frontend CIMD for origin aliases ([#2186](https://github.com/chattocorp/chatto/issues/2186)) ([3bc4dd1](https://github.com/chattocorp/chatto/commit/3bc4dd196491abe479da7c6189719a06dbe3a223))
* **frontend:** divide member search from presence groups ([#2187](https://github.com/chattocorp/chatto/issues/2187)) ([d7c2195](https://github.com/chattocorp/chatto/commit/d7c2195225a551355ce85b472fe4c422bae6d79c))


### Performance Improvements

* **ci:** shorten and harden validation ([#2185](https://github.com/chattocorp/chatto/issues/2185)) ([68d68ba](https://github.com/chattocorp/chatto/commit/68d68ba530c3cdc1825e0241752361d3bbf38254))

## [0.5.0-alpha.1](https://github.com/chattocorp/chatto/compare/v0.4.8...v0.5.0-alpha.1) (2026-08-29)


### ⚠ BREAKING CHANGES

* **authz:** flatten interaction read permission ([#2175](https://github.com/chattocorp/chatto/issues/2175))
* **proto:** separate internal storage contracts ([#2162](https://github.com/chattocorp/chatto/issues/2162))
* **proto:** relocate live-only user payloads ([#2153](https://github.com/chattocorp/chatto/issues/2153))
* **account-deletion:** let admins delete other users ([#2134](https://github.com/chattocorp/chatto/issues/2134))
* **profile:** add rich user profiles and local time ([#2149](https://github.com/chattocorp/chatto/issues/2149))
* **auth:** keep active users signed in ([#2102](https://github.com/chattocorp/chatto/issues/2102))
* **api:** prevent bots from starting DMs ([#2113](https://github.com/chattocorp/chatto/issues/2113))
* **rbac:** require message.read for channel content ([#2100](https://github.com/chattocorp/chatto/issues/2100))
* **auth:** add renewable bearer sessions ([#2092](https://github.com/chattocorp/chatto/issues/2092))
* **auth:** remove legacy cookie sessions ([#2087](https://github.com/chattocorp/chatto/issues/2087))
* **push:** support remote-server web push ([#2055](https://github.com/chattocorp/chatto/issues/2055))
* **notifications:** replace Notifications 1.0 with persistent notifications ([#2061](https://github.com/chattocorp/chatto/issues/2061))
* **notifications:** replace Notifications 1.0 with persistent notifications ([#2020](https://github.com/chattocorp/chatto/issues/2020))
* **auth:** require CIMD clients for remote access ([#2013](https://github.com/chattocorp/chatto/issues/2013))
* **search:** rank cross-server Cmd-K results ([#1862](https://github.com/chattocorp/chatto/issues/1862))
* **cli:** remove passphrase argument flags ([#1705](https://github.com/chattocorp/chatto/issues/1705))
* **metrics:** retire legacy service inventory ([#1703](https://github.com/chattocorp/chatto/issues/1703))
* **video:** add seekable HLS playback ([#1624](https://github.com/chattocorp/chatto/issues/1624))
* **realtime:** add resumable server projection stream ([#1588](https://github.com/chattocorp/chatto/issues/1588))

### Features

* **account-deletion:** let admins delete other users ([#2134](https://github.com/chattocorp/chatto/issues/2134)) ([37c9d89](https://github.com/chattocorp/chatto/commit/37c9d89d0d0cba8996b5948c906d261f37c6f76a))
* add native social post previews ([#1569](https://github.com/chattocorp/chatto/issues/1569)) ([3a1a72f](https://github.com/chattocorp/chatto/commit/3a1a72fe88228faa335622eba09977d0b43a53a6))
* **admin:** expose asset cleanup health ([#1401](https://github.com/chattocorp/chatto/issues/1401)) ([66d7de3](https://github.com/chattocorp/chatto/commit/66d7de341a70a49cbe8a3611b3338eb6046a52b5))
* **admin:** report durable worker health ([#1979](https://github.com/chattocorp/chatto/issues/1979)) ([cb697c9](https://github.com/chattocorp/chatto/commit/cb697c9ed58980ac36d9d8233eb03ef342fc0d8c))
* **api:** add client-server compatibility discovery ([#1586](https://github.com/chattocorp/chatto/issues/1586)) ([13e5318](https://github.com/chattocorp/chatto/commit/13e53185f76c2d108f8c1bfa68f14f8ece897a8f))
* **api:** support GET server discovery ([#1396](https://github.com/chattocorp/chatto/issues/1396)) ([cf1a373](https://github.com/chattocorp/chatto/commit/cf1a3736f807b060757a93c83e93f4bf744b1c2d))
* **appconfig:** extract shared configuration loading ([#1856](https://github.com/chattocorp/chatto/issues/1856)) ([732d7b2](https://github.com/chattocorp/chatto/commit/732d7b22aab7dff63cb4345a5a3e8893d41b57bf))
* **auth:** add CIMD-enabled development stack ([#1881](https://github.com/chattocorp/chatto/issues/1881)) ([5e0f471](https://github.com/chattocorp/chatto/commit/5e0f471459141152ff2130757b120c597a31119b))
* **auth:** add renewable bearer sessions ([#2092](https://github.com/chattocorp/chatto/issues/2092)) ([af87cc1](https://github.com/chattocorp/chatto/commit/af87cc171d70aa480869cef6d2162872753e6afa))
* **auth:** add server invite links ([#1983](https://github.com/chattocorp/chatto/issues/1983)) ([ce6b525](https://github.com/chattocorp/chatto/commit/ce6b5252fc5a1891fef96ec85c22d8115468e12f))
* **auth:** allow operators to disable password login ([#2042](https://github.com/chattocorp/chatto/issues/2042)) ([1df8f3e](https://github.com/chattocorp/chatto/commit/1df8f3e175098a86384fca264697f40e85d546ba))
* **auth:** identify OAuth clients through CIMD ([#2012](https://github.com/chattocorp/chatto/issues/2012)) ([8ac44de](https://github.com/chattocorp/chatto/commit/8ac44de281496f7372f7db3b5682c5e3e9b4bc12))
* **auth:** manage member-authorized OAuth clients ([#2014](https://github.com/chattocorp/chatto/issues/2014)) ([2e6f1d0](https://github.com/chattocorp/chatto/commit/2e6f1d044c9af42e3ecec20b2c70d2a21462f031))
* **auth:** require CIMD clients for remote access ([#2013](https://github.com/chattocorp/chatto/issues/2013)) ([792f9bb](https://github.com/chattocorp/chatto/commit/792f9bb218d62722aa485e599d05a56585d88eec))
* **auth:** transfer Authling profiles to Chatto ([#2076](https://github.com/chattocorp/chatto/issues/2076)) ([c91cac7](https://github.com/chattocorp/chatto/commit/c91cac7ecb880669f14a24a261c38f0bfcf035d5))
* **authz:** add interaction-scoped message reads ([#2114](https://github.com/chattocorp/chatto/issues/2114)) ([06396c1](https://github.com/chattocorp/chatto/commit/06396c112644f5c88d2029a317a0197c28b9991b))
* **bots:** activate bots through notifications ([#2127](https://github.com/chattocorp/chatto/issues/2127)) ([71b4577](https://github.com/chattocorp/chatto/commit/71b45773e6b4c39a1dccbf862c4f3cd406f348b9))
* **bots:** add baseline bot accounts ([#2060](https://github.com/chattocorp/chatto/issues/2060)) ([3a66bcd](https://github.com/chattocorp/chatto/commit/3a66bcd57427a9e5933c4ccd9e933fcb5113b951))
* **bots:** add multiple incoming webhooks ([#2137](https://github.com/chattocorp/chatto/issues/2137)) ([80a1126](https://github.com/chattocorp/chatto/commit/80a11260916cf34da19cde521027b86327e49f50))
* **bots:** support owner reassignment ([#2085](https://github.com/chattocorp/chatto/issues/2085)) ([d8d3dd9](https://github.com/chattocorp/chatto/commit/d8d3dd96d8a6ff12a03fb84e37692f1a928a08d1))
* **calls:** show lifecycle events in room timelines ([#2032](https://github.com/chattocorp/chatto/issues/2032)) ([033130d](https://github.com/chattocorp/chatto/commit/033130d060d267e3f6dcfe822d2a1967f1fb982c))
* **core:** accelerate projection startup with encrypted snapshots ([#1549](https://github.com/chattocorp/chatto/issues/1549)) ([0a1417d](https://github.com/chattocorp/chatto/commit/0a1417da3080f56b970cfb89215087bdb0cd0254))
* **core:** add encrypted projection snapshot canary ([#1488](https://github.com/chattocorp/chatto/issues/1488)) ([835737e](https://github.com/chattocorp/chatto/commit/835737e567a03cd2901f8e1a6f1d36d6f59373bc))
* **core:** bound projection snapshot cleanup ([#1538](https://github.com/chattocorp/chatto/issues/1538)) ([3e332d3](https://github.com/chattocorp/chatto/commit/3e332d3874f562afd9d3511f3822aef2d5e96bc5))
* **datacrypto:** extract shared encryption primitives ([#1853](https://github.com/chattocorp/chatto/issues/1853)) ([4235d7c](https://github.com/chattocorp/chatto/commit/4235d7c0b2fc68ca4393db9cc03d0b88d23c404f))
* **desktop:** adapt game streams with simulcast ([#2028](https://github.com/chattocorp/chatto/issues/2028)) ([dda3eb7](https://github.com/chattocorp/chatto/commit/dda3eb7fa88ea469573edd00d28e0ff8ad62666b))
* **desktop:** add experimental Deno CEF app ([#1893](https://github.com/chattocorp/chatto/issues/1893)) ([ab25ca8](https://github.com/chattocorp/chatto/commit/ab25ca858f336965ad68ea943b0622bd92507373))
* **desktop:** replace Deno Desktop with Electron ([#1939](https://github.com/chattocorp/chatto/issues/1939)) ([362204f](https://github.com/chattocorp/chatto/commit/362204f540f77d0d4645c92fbdd75eb4a80e4b5e))
* **desktop:** sign and notarise macOS releases ([#2048](https://github.com/chattocorp/chatto/issues/2048)) ([a0b4c0e](https://github.com/chattocorp/chatto/commit/a0b4c0e8f4ed2aac442fd50f922293deba931dcb))
* **desktop:** sign Windows releases ([#2050](https://github.com/chattocorp/chatto/issues/2050)) ([6f0c875](https://github.com/chattocorp/chatto/commit/6f0c875788622829a3643427f8c4161a1aff4629))
* **desktop:** stream macOS games through LiveKit ([#2024](https://github.com/chattocorp/chatto/issues/2024)) ([ff3842b](https://github.com/chattocorp/chatto/commit/ff3842bfe36819fbdfbbc61e418ddd40892981f5))
* **desktop:** unify native and browser screen sharing ([#2034](https://github.com/chattocorp/chatto/issues/2034)) ([59aee59](https://github.com/chattocorp/chatto/commit/59aee59e6bd5242425dcbb02cf1355fc5d7921d3))
* **dev:** add bind-mounted Compose watch stack ([#2038](https://github.com/chattocorp/chatto/issues/2038)) ([ab945c9](https://github.com/chattocorp/chatto/commit/ab945c9ec3f207ddb8bb837c9d0813b7fce322e3))
* **dev:** add compiled Conductor run task ([#1696](https://github.com/chattocorp/chatto/issues/1696)) ([6c3b83b](https://github.com/chattocorp/chatto/commit/6c3b83b874660fe71a0eeb77e0603b155ddaeb13))
* **dev:** replace Compose stack with Pitchfork ([#2047](https://github.com/chattocorp/chatto/issues/2047)) ([41f4424](https://github.com/chattocorp/chatto/commit/41f4424fa08a884d0637c90f6c840fef5c9b1900))
* **docs:** publish stable and development channels ([#1490](https://github.com/chattocorp/chatto/issues/1490)) ([67e15b1](https://github.com/chattocorp/chatto/commit/67e15b1e0a41b0474173d6d666cf636b6a6b293c))
* **email:** add JMAP delivery transport ([#1443](https://github.com/chattocorp/chatto/issues/1443)) ([49a090d](https://github.com/chattocorp/chatto/commit/49a090dbe6d383f18c33ea8ee56a02c4fd8b66a8))
* **events:** add durable pull-worker execution ([#1972](https://github.com/chattocorp/chatto/issues/1972)) ([8777064](https://github.com/chattocorp/chatto/commit/877706486ffba9286fc2c5961eb661485b248ffa))
* **events:** add selectable mutation consistency boundaries ([#1962](https://github.com/chattocorp/chatto/issues/1962)) ([3db6575](https://github.com/chattocorp/chatto/commit/3db65758a8b68e4d0e1373d51790e8afd374533a))
* **events:** extract shared framework module ([#1833](https://github.com/chattocorp/chatto/issues/1833)) ([3195ffd](https://github.com/chattocorp/chatto/commit/3195ffd57336dc95196c6d539e0f13aeee087376))
* **frontend:** add Arabic and Hebrew translations ([#1929](https://github.com/chattocorp/chatto/issues/1929)) ([3ca9bc4](https://github.com/chattocorp/chatto/commit/3ca9bc4b04873aec1caecf2592fd1afb81899a47))
* **frontend:** add Chatto laser clicker easter egg ([#1605](https://github.com/chattocorp/chatto/issues/1605)) ([8b5119d](https://github.com/chattocorp/chatto/commit/8b5119d514d7455b465d0aead42bde51e8e35f9c))
* **frontend:** add Chinese translations ([#1745](https://github.com/chattocorp/chatto/issues/1745)) ([0c3b74b](https://github.com/chattocorp/chatto/commit/0c3b74b87e2d49dacb40ade164535c94a06fc45d))
* **frontend:** add composer formatting shortcuts ([#2103](https://github.com/chattocorp/chatto/issues/2103)) ([d7e36ad](https://github.com/chattocorp/chatto/commit/d7e36ad13d305700ffdfc671d5e8ecbf564ec8f3))
* **frontend:** add context menus for visible rooms ([#1637](https://github.com/chattocorp/chatto/issues/1637)) ([9dbb684](https://github.com/chattocorp/chatto/commit/9dbb684c1b46e801b3230bfa6440600a306057c1))
* **frontend:** add context-menu copy actions ([#2090](https://github.com/chattocorp/chatto/issues/2090)) ([7cc495d](https://github.com/chattocorp/chatto/commit/7cc495d6998ad80fba9094d9f1f17ce0fde648d0))
* **frontend:** add inline message timestamps ([#1558](https://github.com/chattocorp/chatto/issues/1558)) ([70fe404](https://github.com/chattocorp/chatto/commit/70fe404126f39a541d47bdebb0c346c9d27cfa77))
* **frontend:** add regional English locales ([#1532](https://github.com/chattocorp/chatto/issues/1532)) ([adea7d1](https://github.com/chattocorp/chatto/commit/adea7d160afe2df3012bbb9249d2daabfe72e402))
* **frontend:** add responsive resizable thread pane ([#1927](https://github.com/chattocorp/chatto/issues/1927)) ([76e4757](https://github.com/chattocorp/chatto/commit/76e475783a12140f74c2a8fb429db30c933aebde))
* **frontend:** add room group context menu ([#1672](https://github.com/chattocorp/chatto/issues/1672)) ([942d117](https://github.com/chattocorp/chatto/commit/942d117d1999b71ea412e7cb9c05b18c7ff05f5e))
* **frontend:** add room-scoped message search ([#1863](https://github.com/chattocorp/chatto/issues/1863)) ([0848a50](https://github.com/chattocorp/chatto/commit/0848a508c3d71c2396c7de702a7f300c6a86c3ec))
* **frontend:** add server and room context menus ([#1580](https://github.com/chattocorp/chatto/issues/1580)) ([fea2d34](https://github.com/chattocorp/chatto/commit/fea2d34f6363dc2fc6694142c46c56edb213b9d4))
* **frontend:** add six language translations ([#1908](https://github.com/chattocorp/chatto/issues/1908)) ([1787ded](https://github.com/chattocorp/chatto/commit/1787ded51a68a3b83dedd53351b1a90687954ff1))
* **frontend:** add visual and Markdown composers ([#2078](https://github.com/chattocorp/chatto/issues/2078)) ([fc7b4be](https://github.com/chattocorp/chatto/commit/fc7b4be735d01769ef3cbbe8793d5e4d159c70a1))
* **frontend:** adopt TanStack Query for snapshot reads ([#1870](https://github.com/chattocorp/chatto/issues/1870)) ([c747f83](https://github.com/chattocorp/chatto/commit/c747f83a5712a4048f7125a89f28603cfd014981))
* **frontend:** animate segmented control selection ([#1938](https://github.com/chattocorp/chatto/issues/1938)) ([b52e944](https://github.com/chattocorp/chatto/commit/b52e9440a60312de1c2ad3ab4ef6171b9371f6e5))
* **frontend:** animate video processing state ([#1718](https://github.com/chattocorp/chatto/issues/1718)) ([ba68708](https://github.com/chattocorp/chatto/commit/ba68708bf2f7c698ea23387c430ff6179a23951f))
* **frontend:** authenticate remote servers in a popup ([#1560](https://github.com/chattocorp/chatto/issues/1560)) ([8d5354c](https://github.com/chattocorp/chatto/commit/8d5354cd6203f2f6175748099cce75601cd3ffbf))
* **frontend:** consolidate app preferences into settings ([#2126](https://github.com/chattocorp/chatto/issues/2126)) ([772e0f2](https://github.com/chattocorp/chatto/commit/772e0f2d44c4bc1ddafe5b307c5f9de48b9cd4a8))
* **frontend:** copy message text from action menus ([#1598](https://github.com/chattocorp/chatto/issues/1598)) ([4e91f4e](https://github.com/chattocorp/chatto/commit/4e91f4ef325f1af0df0466e6e593d9540cf39c73))
* **frontend:** debounce server message search ([#1867](https://github.com/chattocorp/chatto/issues/1867)) ([81fb048](https://github.com/chattocorp/chatto/commit/81fb048a919007a3640e35000321eb83049c2d5e))
* **frontend:** enforce Content Security Policy ([#2143](https://github.com/chattocorp/chatto/issues/2143)) ([2b8a2f8](https://github.com/chattocorp/chatto/commit/2b8a2f85ed21aeac15cfc16e12fc3d65bab43532))
* **frontend:** expand lazy-loaded locale support ([#1537](https://github.com/chattocorp/chatto/issues/1537)) ([ffc9470](https://github.com/chattocorp/chatto/commit/ffc9470a06ce2242556779f8510fa4bdaf0b09a1))
* **frontend:** frame preference forms ([#2104](https://github.com/chattocorp/chatto/issues/2104)) ([5b79973](https://github.com/chattocorp/chatto/commit/5b79973a2d7d8609fd8192274a5fc265df3f9c2b))
* **frontend:** highlight rich composer mode ([#1412](https://github.com/chattocorp/chatto/issues/1412)) ([468cf14](https://github.com/chattocorp/chatto/commit/468cf146fdaab490c8e703a10577699554ff170d))
* **frontend:** improve pinned message presentation ([#2016](https://github.com/chattocorp/chatto/issues/2016)) ([2f566ff](https://github.com/chattocorp/chatto/commit/2f566ffe0db99eefef3a8c13c34c0132414841ea))
* **frontend:** keep screen awake during calls ([#2054](https://github.com/chattocorp/chatto/issues/2054)) ([98b0d9f](https://github.com/chattocorp/chatto/commit/98b0d9fdfa53fb034a46806d478b0122dc9279ff))
* **frontend:** link user menu to server admin ([#2147](https://github.com/chattocorp/chatto/issues/2147)) ([a2944f5](https://github.com/chattocorp/chatto/commit/a2944f5e4c184a6ed881a254c0015ad5a4b33c54))
* **frontend:** open created threads automatically ([#2108](https://github.com/chattocorp/chatto/issues/2108)) ([4b909a1](https://github.com/chattocorp/chatto/commit/4b909a1ed85cbdae46a955794c630bf83258fc0d))
* **frontend:** open profiles in direct messages ([#2164](https://github.com/chattocorp/chatto/issues/2164)) ([023bab1](https://github.com/chattocorp/chatto/commit/023bab1bfe2470b78a2f4d61230cc8c891842a68))
* **frontend:** polish navigation feedback ([#2166](https://github.com/chattocorp/chatto/issues/2166)) ([2707586](https://github.com/chattocorp/chatto/commit/27075868dd036e48cf04da9ce44fa1cbb04b4d50))
* **frontend:** prepare client shell for RTL locales ([#1910](https://github.com/chattocorp/chatto/issues/1910)) ([06e4e24](https://github.com/chattocorp/chatto/commit/06e4e24e5b71fc96c4e7a2281075158206a69b06))
* **frontend:** refine permissions and scroll UI ([#1685](https://github.com/chattocorp/chatto/issues/1685)) ([8a32ee0](https://github.com/chattocorp/chatto/commit/8a32ee01df7e25c040d61ce0eb0a6a48a3ab7a5c))
* **frontend:** refine room and thread dialogs ([#2106](https://github.com/chattocorp/chatto/issues/2106)) ([5601b70](https://github.com/chattocorp/chatto/commit/5601b70a3128be21bcbeed49148f1b0abd1777a7))
* **frontend:** refine room group sidebar sections ([#2037](https://github.com/chattocorp/chatto/issues/2037)) ([58e9daf](https://github.com/chattocorp/chatto/commit/58e9dafa9704ba1d3cc952f13082458c9375d6e7))
* **frontend:** refresh semantic colors and buttons ([#2159](https://github.com/chattocorp/chatto/issues/2159)) ([9d611a9](https://github.com/chattocorp/chatto/commit/9d611a982f1aef534db26994593f97abd0edeab9))
* **frontend:** remove implicit auto-away presence ([#2136](https://github.com/chattocorp/chatto/issues/2136)) ([2a0f470](https://github.com/chattocorp/chatto/commit/2a0f47057c91674d3f53935db40b8724d012feb6))
* **frontend:** render GFM tables ([#1612](https://github.com/chattocorp/chatto/issues/1612)) ([c36644b](https://github.com/chattocorp/chatto/commit/c36644b2fe21a4902c3f1b5b09cb4f6e4d49a2fd))
* **frontend:** replace Paraglide with Lingua ([#1918](https://github.com/chattocorp/chatto/issues/1918)) ([c55ea79](https://github.com/chattocorp/chatto/commit/c55ea79ca70a093564a4c71a19b2c1e4a2947c2f))
* **frontend:** restructure settings and preferences ([#2097](https://github.com/chattocorp/chatto/issues/2097)) ([53599fb](https://github.com/chattocorp/chatto/commit/53599fb49293290d2d06a14a1839a9b8cb90726c))
* **frontend:** show attachment upload progress ([#1689](https://github.com/chattocorp/chatto/issues/1689)) ([d6cb466](https://github.com/chattocorp/chatto/commit/d6cb466e1092e2825fd173d943579cd547222bfb))
* **frontend:** show inline message state markers ([#2110](https://github.com/chattocorp/chatto/issues/2110)) ([53712e2](https://github.com/chattocorp/chatto/commit/53712e2fc86c6e584a73bf435d99e6c8df0fc919))
* **frontend:** show server re-login status ([#2105](https://github.com/chattocorp/chatto/issues/2105)) ([5111c2e](https://github.com/chattocorp/chatto/commit/5111c2e0248f771e47c55b3e97c98ea88adffa2e))
* **frontend:** show user ID in account settings ([#1921](https://github.com/chattocorp/chatto/issues/1921)) ([f189952](https://github.com/chattocorp/chatto/commit/f18995220e9a84167f5df54f388834338a0056c4))
* **frontend:** smooth sliding pane motion ([#1936](https://github.com/chattocorp/chatto/issues/1936)) ([74859a7](https://github.com/chattocorp/chatto/commit/74859a74223f3df95c4e1b4a79fc545965009dab))
* **frontend:** submit dialog actions with Enter ([#2107](https://github.com/chattocorp/chatto/issues/2107)) ([5f19ecf](https://github.com/chattocorp/chatto/commit/5f19ecf193757d914c026216257b46da2c93bd60))
* **frontend:** suppress previews for angle-bracket links ([#2144](https://github.com/chattocorp/chatto/issues/2144)) ([815299b](https://github.com/chattocorp/chatto/commit/815299beefff5e8614ead189022b4322439d6e4f))
* **frontend:** unify collapsible sidebar sections ([#2045](https://github.com/chattocorp/chatto/issues/2045)) ([c8f9ec9](https://github.com/chattocorp/chatto/commit/c8f9ec9b687cee17cd7e58e8c6484bedf9881978))
* **frontend:** unify interactive control styling ([#1708](https://github.com/chattocorp/chatto/issues/1708)) ([135c94e](https://github.com/chattocorp/chatto/commit/135c94e3de15b2990d3cf2f76db7e9ffbe4518a1))
* **frontend:** unify message action menus ([#1811](https://github.com/chattocorp/chatto/issues/1811)) ([38fe1cb](https://github.com/chattocorp/chatto/commit/38fe1cb32ff6a8c8e4412ed76c862109bbfa5de3))
* **management:** add resource-scoped room administration ([#1630](https://github.com/chattocorp/chatto/issues/1630)) ([1647794](https://github.com/chattocorp/chatto/commit/1647794928e0df792256b837e89e479485b10d03))
* **messages:** add pinned channel messages ([#1990](https://github.com/chattocorp/chatto/issues/1990)) ([2b3b187](https://github.com/chattocorp/chatto/commit/2b3b18736ed1ef18249f2b1cb0b472cf92a83ee3))
* **natsruntime:** extract embedded NATS lifecycle ([#1845](https://github.com/chattocorp/chatto/issues/1845)) ([cb24279](https://github.com/chattocorp/chatto/commit/cb2427958ec1621d7307a43f72f72f98e7332ebd))
* **notifications:** add Badge and room-message delivery ([#2119](https://github.com/chattocorp/chatto/issues/2119)) ([3dde347](https://github.com/chattocorp/chatto/commit/3dde347c10da86f088e602a42babab853cb204a8))
* **notifications:** add scoped policy matrix ([#2111](https://github.com/chattocorp/chatto/issues/2111)) ([631c1f5](https://github.com/chattocorp/chatto/commit/631c1f5456f02862c2dfe0fa77a565d1a171b0ce))
* **notifications:** add test push action ([#1480](https://github.com/chattocorp/chatto/issues/1480)) ([add1590](https://github.com/chattocorp/chatto/commit/add1590f82d55627373c9c3585329792ad8da0bc))
* **notifications:** replace Notifications 1.0 with persistent notifications ([#2020](https://github.com/chattocorp/chatto/issues/2020)) ([29c88d9](https://github.com/chattocorp/chatto/commit/29c88d98d495f886fefd8c64948a0826293a066f))
* **notifications:** replace Notifications 1.0 with persistent notifications ([#2061](https://github.com/chattocorp/chatto/issues/2061)) ([1f63f0f](https://github.com/chattocorp/chatto/commit/1f63f0ffa420ba0189dffdb4e115f1dc9ee6afd6))
* **profile:** add rich user profiles and local time ([#2149](https://github.com/chattocorp/chatto/issues/2149)) ([1203310](https://github.com/chattocorp/chatto/commit/12033102f290ef4ceb915a284970d98b9534edcc))
* **push:** support remote-server web push ([#2055](https://github.com/chattocorp/chatto/issues/2055)) ([4f7fbfc](https://github.com/chattocorp/chatto/commit/4f7fbfcd05759cc93a13046e90ba62698fa64dcf))
* **rbac:** require message.read for channel content ([#2100](https://github.com/chattocorp/chatto/issues/2100)) ([f340f3b](https://github.com/chattocorp/chatto/commit/f340f3bb21f94a6a4ee36f0646c862bc3bb971bf))
* **rbac:** treat everyone as scoped permission baseline ([#1614](https://github.com/chattocorp/chatto/issues/1614)) ([0446d01](https://github.com/chattocorp/chatto/commit/0446d015dabdb7d946e4d28d11ae87a1ddd4d52c))
* **realtime:** add resumable server projection stream ([#1588](https://github.com/chattocorp/chatto/issues/1588)) ([a886853](https://github.com/chattocorp/chatto/commit/a88685314420cdcc0824cb44777c68c02ed9db41))
* **rooms:** add joinable room previews ([#1546](https://github.com/chattocorp/chatto/issues/1546)) ([0e96593](https://github.com/chattocorp/chatto/commit/0e96593ec67ee3f6f2212e876d5d29af814ca330))
* **rooms:** add slow mode ([#1980](https://github.com/chattocorp/chatto/issues/1980)) ([fd385b2](https://github.com/chattocorp/chatto/commit/fd385b24cb990175c5b748065f7638d8706b0c76))
* **rooms:** allow flexible Unicode names ([#1986](https://github.com/chattocorp/chatto/issues/1986)) ([4b5d84c](https://github.com/chattocorp/chatto/commit/4b5d84cc9b6c46f9f919fcda432c7ef22e3e9713))
* **rooms:** manage channel room members ([#1713](https://github.com/chattocorp/chatto/issues/1713)) ([2125b37](https://github.com/chattocorp/chatto/commit/2125b37257e2c4d1455200c0dfb8ab17ad1371ba))
* **rooms:** support Unicode room names ([#1739](https://github.com/chattocorp/chatto/issues/1739)) ([1d1e850](https://github.com/chattocorp/chatto/commit/1d1e850d57a3838d82a8ff1227860cfbfe77fced))
* **search:** add pluggable message search ([#1632](https://github.com/chattocorp/chatto/issues/1632)) ([a9e1224](https://github.com/chattocorp/chatto/commit/a9e1224167748da18895c9a4fa4051ce8a11b02a))
* **search:** rank cross-server Cmd-K results ([#1862](https://github.com/chattocorp/chatto/issues/1862)) ([73912ad](https://github.com/chattocorp/chatto/commit/73912ad893d0f995b274cb338575899b5d498285))
* **search:** support filter-only queries ([#1868](https://github.com/chattocorp/chatto/issues/1868)) ([0b14c9a](https://github.com/chattocorp/chatto/commit/0b14c9a01ca2c3454ebb417293681e3de400c206))
* **shields:** add Shields.io community badges ([#1467](https://github.com/chattocorp/chatto/issues/1467)) ([c6b5752](https://github.com/chattocorp/chatto/commit/c6b575271adb864097e24992097e307f874c2d6e))
* **threading:** add configurable room threading modes ([#2091](https://github.com/chattocorp/chatto/issues/2091)) ([cf04c6d](https://github.com/chattocorp/chatto/commit/cf04c6d021d8208fd984369005b4dd68802904c1))
* **threads:** let authors create threads with root messages ([#1906](https://github.com/chattocorp/chatto/issues/1906)) ([55ace4e](https://github.com/chattocorp/chatto/commit/55ace4e23c686f2bb2f7982de9fb58de4664a0d6))
* **video:** add durable asset processing workers ([#1931](https://github.com/chattocorp/chatto/issues/1931)) ([9d66dc2](https://github.com/chattocorp/chatto/commit/9d66dc2c1eb6c89d5839dfaccdb7e753bac2bf5a))
* **video:** add seekable HLS playback ([#1624](https://github.com/chattocorp/chatto/issues/1624)) ([1c07057](https://github.com/chattocorp/chatto/commit/1c0705725a8c15d957e551a8ea47c77a69912e20))


### Bug Fixes

* **api:** batch presence hydration ([#1597](https://github.com/chattocorp/chatto/issues/1597)) ([8812cb9](https://github.com/chattocorp/chatto/commit/8812cb9811318c2f40b31248e11122a8f252acb1))
* **api:** omit deleted users from room member reads ([#1964](https://github.com/chattocorp/chatto/issues/1964)) ([793d41c](https://github.com/chattocorp/chatto/commit/793d41c09635a5c8bf628df656db474038fcd35a))
* **api:** prevent bots from starting DMs ([#2113](https://github.com/chattocorp/chatto/issues/2113)) ([50d6d38](https://github.com/chattocorp/chatto/commit/50d6d3884df69ec7d7624cb9c0653ffa6345bdc3))
* **api:** redact password verifiers from audit payloads ([#2039](https://github.com/chattocorp/chatto/issues/2039)) ([f1eed9b](https://github.com/chattocorp/chatto/commit/f1eed9b938868f9b85fc9090b5b256cb384eb007))
* **assets:** mark NATS streams as non-seekable ([#1607](https://github.com/chattocorp/chatto/issues/1607)) ([96febb8](https://github.com/chattocorp/chatto/commit/96febb829379a7e1a8353e3a664f45a6e13774b5))
* **auth:** allow configured browser origin aliases ([#2123](https://github.com/chattocorp/chatto/issues/2123)) ([6dba2ee](https://github.com/chattocorp/chatto/commit/6dba2eecf201bce970e890019aa90c45c864286d))
* **auth:** keep active users signed in ([#2102](https://github.com/chattocorp/chatto/issues/2102)) ([7227c87](https://github.com/chattocorp/chatto/commit/7227c8783f17ea4d47d13974f6879108cf0ad44c))
* **auth:** preserve legacy browser login ([#2120](https://github.com/chattocorp/chatto/issues/2120)) ([65f929b](https://github.com/chattocorp/chatto/commit/65f929bb071d2d6730df43bbfefa95fe2db75424))
* **calls:** share browser tab audio ([4f27dd8](https://github.com/chattocorp/chatto/commit/4f27dd8f50b0ee8d127de3cc6260328d42db196e))
* **calls:** show active DM calls in sidebar ([#1676](https://github.com/chattocorp/chatto/issues/1676)) ([d894882](https://github.com/chattocorp/chatto/commit/d894882198ead26cbc2a0a93bce6095b8c13e0ad))
* **ci:** publish desktop releases without checkout ([#2033](https://github.com/chattocorp/chatto/issues/2033)) ([45f8a16](https://github.com/chattocorp/chatto/commit/45f8a1631cbdc81fca41c688f45bf2e67e405295))
* **ci:** time out media environment setup ([#1486](https://github.com/chattocorp/chatto/issues/1486)) ([6e2373f](https://github.com/chattocorp/chatto/commit/6e2373f87074f406d47bf187f062fbe9e9b49bfb))
* **config:** configure API response compression ([#1502](https://github.com/chattocorp/chatto/issues/1502)) ([443f26f](https://github.com/chattocorp/chatto/commit/443f26f8ed24578eddc3d82f193d4b44ca557bd0))
* **core:** decrypt projected user PII on demand ([#1551](https://github.com/chattocorp/chatto/issues/1551)) ([879c39d](https://github.com/chattocorp/chatto/commit/879c39ddc7014fbcd728d3798cb685d6a67b83ec))
* **core:** enforce exclusive asset attachments ([#2041](https://github.com/chattocorp/chatto/issues/2041)) ([ee40655](https://github.com/chattocorp/chatto/commit/ee40655dadf250f957fb3b05be34bee44ac51598))
* **core:** fence message lifecycle mutations ([#1925](https://github.com/chattocorp/chatto/issues/1925)) ([5c01511](https://github.com/chattocorp/chatto/commit/5c015112396ee0c8838139d75448376f39d924bd))
* **core:** forbid threads in direct messages ([#1583](https://github.com/chattocorp/chatto/issues/1583)) ([896c74b](https://github.com/chattocorp/chatto/commit/896c74b0fff0a2dba614acc5202e04236b33151b))
* **core:** harden CallModel state ownership ([#1771](https://github.com/chattocorp/chatto/issues/1771)) ([b698e72](https://github.com/chattocorp/chatto/commit/b698e7250f74b205bb84c189f823f51a28036c43))
* **core:** make user key shredding crash-safe ([#1973](https://github.com/chattocorp/chatto/issues/1973)) ([63f014f](https://github.com/chattocorp/chatto/commit/63f014f1e41ccc2ab7d7bd48393fe477c16eb738))
* **core:** migrate projection snapshot pointer lineage ([#1572](https://github.com/chattocorp/chatto/issues/1572)) ([c6ca943](https://github.com/chattocorp/chatto/commit/c6ca9438d85e01475f89144de4095726470fc64b))
* **core:** preserve cleared RBAC defaults ([#1543](https://github.com/chattocorp/chatto/issues/1543)) ([be7b4d9](https://github.com/chattocorp/chatto/commit/be7b4d912b50e2b45de924f6f078e30472e5e353))
* **core:** publish room layout after projection catch-up ([#1966](https://github.com/chattocorp/chatto/issues/1966)) ([ccc31f9](https://github.com/chattocorp/chatto/commit/ccc31f9166bc651b80bfda8b84c809ac4d23e439))
* **core:** re-authorize message mutations on OCC retries ([#1960](https://github.com/chattocorp/chatto/issues/1960)) ([6df5f6e](https://github.com/chattocorp/chatto/commit/6df5f6e60f54b522ba7cbcd8c1504ddb9c5d0699))
* **core:** recheck message authorization during OCC ([#1922](https://github.com/chattocorp/chatto/issues/1922)) ([43219b3](https://github.com/chattocorp/chatto/commit/43219b305df7c9814b96dca801bf4f0b3da6b0f2))
* **core:** recover safely after NATS outages ([#1891](https://github.com/chattocorp/chatto/issues/1891)) ([2fd9674](https://github.com/chattocorp/chatto/commit/2fd9674944f89c48a58cdb077ef48e3f8838d474))
* **core:** retry link previews across validated IPs ([#1577](https://github.com/chattocorp/chatto/issues/1577)) ([1fc1a19](https://github.com/chattocorp/chatto/commit/1fc1a198b0a6326710a764a8e21b2126c96d6802))
* **desktop:** identify bundled frontend builds ([#2036](https://github.com/chattocorp/chatto/issues/2036)) ([cb8ebd8](https://github.com/chattocorp/chatto/commit/cb8ebd88a14edb6fd92cfc92bd1f346d9726d8cb))
* **desktop:** make release verification reliable ([#1902](https://github.com/chattocorp/chatto/issues/1902)) ([898eec2](https://github.com/chattocorp/chatto/commit/898eec243b2a27d35068340bd5979eeaf48f7bff))
* **desktop:** require immutable GitHub OIDC subjects ([#2051](https://github.com/chattocorp/chatto/issues/2051)) ([fdb260d](https://github.com/chattocorp/chatto/commit/fdb260d86c5f3c9c7e111892890414fe3fb8c0f7))
* **dev:** bootstrap alice and bob in compose ([#1907](https://github.com/chattocorp/chatto/issues/1907)) ([d67b4da](https://github.com/chattocorp/chatto/commit/d67b4da405a256ac4514fe63d2a651f72c615da6))
* **dev:** build Lingua during workspace setup ([#1951](https://github.com/chattocorp/chatto/issues/1951)) ([c23205d](https://github.com/chattocorp/chatto/commit/c23205df31dfffda756cb73998da63819fe868b4))
* **dev:** clean up archived workspace processes ([#2121](https://github.com/chattocorp/chatto/issues/2121)) ([39100e8](https://github.com/chattocorp/chatto/commit/39100e8c9fa6f0b4a11219d204bac14125c36f4e))
* **dev:** clean up Compose stack on archive ([#1932](https://github.com/chattocorp/chatto/issues/1932)) ([5260dd7](https://github.com/chattocorp/chatto/commit/5260dd70af951a4f59608cdd9d8ef61b07c9a39a))
* **dev:** isolate Pitchfork workspace services ([#2067](https://github.com/chattocorp/chatto/issues/2067)) ([967f0cc](https://github.com/chattocorp/chatto/commit/967f0cca67eeae42a19723867f5a0416e6c898eb))
* **dev:** keep Conductor tasks in process group ([#1699](https://github.com/chattocorp/chatto/issues/1699)) ([483f802](https://github.com/chattocorp/chatto/commit/483f8022e6ddb7637aad0621d587a3952f4bc4dc))
* **dev:** release Conductor workspace ports ([#1704](https://github.com/chattocorp/chatto/issues/1704)) ([356ca89](https://github.com/chattocorp/chatto/commit/356ca897e1571bdfc67ba90c01a8f4e08a683331))
* **dev:** restore mise chatto command routing ([#1707](https://github.com/chattocorp/chatto/issues/1707)) ([8781cbe](https://github.com/chattocorp/chatto/commit/8781cbeebfad029c89ecdfc4e0ecf6b1ee4865bd))
* **dm:** hide empty conversations until first message ([#1635](https://github.com/chattocorp/chatto/issues/1635)) ([56b1916](https://github.com/chattocorp/chatto/commit/56b1916dd755c9e41c0587b4eb6ceefc91b38c68))
* **dockercompose:** restart services unless stopped ([#2052](https://github.com/chattocorp/chatto/issues/2052)) ([0fcd020](https://github.com/chattocorp/chatto/commit/0fcd020203119140ff32a6e6bc77d431b489a8b9))
* **dockercompose:** use muxed LiveKit UDP port ([#1503](https://github.com/chattocorp/chatto/issues/1503)) ([4974c63](https://github.com/chattocorp/chatto/commit/4974c63d09cb8b0eca13f72ca913f25b86123e35))
* **dockercompose:** validate init-env arguments ([#2023](https://github.com/chattocorp/chatto/issues/2023)) ([f413260](https://github.com/chattocorp/chatto/commit/f413260f7525f8b11c0059d2db921c73da342907))
* **docker:** use real service health checks ([#1761](https://github.com/chattocorp/chatto/issues/1761)) ([58d52d9](https://github.com/chattocorp/chatto/commit/58d52d9e831387263f7fd7d31411ae405cf790b0))
* **events:** add bounded subject reads ([#2002](https://github.com/chattocorp/chatto/issues/2002)) ([1a15202](https://github.com/chattocorp/chatto/commit/1a152024bc07a0483fb194f920c1ddf738714272))
* **events:** require pointer projections ([#2000](https://github.com/chattocorp/chatto/issues/2000)) ([4ee0c2b](https://github.com/chattocorp/chatto/commit/4ee0c2b1bb8c7ffff7c1aca0c76acba848fb2e70))
* **events:** validate snapshot bindings ([#2001](https://github.com/chattocorp/chatto/issues/2001)) ([d323298](https://github.com/chattocorp/chatto/commit/d323298eafb1a056fd736903beaf79bb0514c897))
* forward-port 0.4.9 changes ([#1477](https://github.com/chattocorp/chatto/issues/1477)) ([e7119f7](https://github.com/chattocorp/chatto/commit/e7119f7b72c9fa12ff3f6b12255d3da18f0d7420))
* **frontend:** accept all uploadable composer drops ([#2066](https://github.com/chattocorp/chatto/issues/2066)) ([8f2821a](https://github.com/chattocorp/chatto/commit/8f2821a0358c76c66fdba191c5aa41bee960fa26))
* **frontend:** add direct thread message routes ([#1524](https://github.com/chattocorp/chatto/issues/1524)) ([68bba96](https://github.com/chattocorp/chatto/commit/68bba96b036807e7d90b492120e6374286dd05d3))
* **frontend:** align pane header actions ([#2161](https://github.com/chattocorp/chatto/issues/2161)) ([9210ec8](https://github.com/chattocorp/chatto/commit/9210ec89c738e50ada1f2aacbe80c02c34173aed))
* **frontend:** allow app-wide mobile sidebar swipes ([#1534](https://github.com/chattocorp/chatto/issues/1534)) ([a78110e](https://github.com/chattocorp/chatto/commit/a78110e8331af7971863de1db9182b317890155d))
* **frontend:** avoid blank line when enabling rich mode ([#1548](https://github.com/chattocorp/chatto/issues/1548)) ([09aa9e3](https://github.com/chattocorp/chatto/commit/09aa9e326d28cfd95fd38b9cdac29f73fb6b7c22))
* **frontend:** balance room directory card padding ([#2173](https://github.com/chattocorp/chatto/issues/2173)) ([aac58c8](https://github.com/chattocorp/chatto/commit/aac58c8774ffd28f3d8a1bc46d79daae7871fce9))
* **frontend:** cache PWA assets on demand ([#1786](https://github.com/chattocorp/chatto/issues/1786)) ([ded8061](https://github.com/chattocorp/chatto/commit/ded80619c1e3ec75cda28244c6618662a5261a69))
* **frontend:** collapse invisible markdown spacing ([#1539](https://github.com/chattocorp/chatto/issues/1539)) ([d3c8d19](https://github.com/chattocorp/chatto/commit/d3c8d19449b1fd55c60baa1aa40b362ce62b0450))
* **frontend:** default room sidebar to closed ([#1633](https://github.com/chattocorp/chatto/issues/1633)) ([c751eb7](https://github.com/chattocorp/chatto/commit/c751eb7bff97e7ff05704cfc0800ba1414a42e70))
* **frontend:** enforce mono voice call audio ([#1489](https://github.com/chattocorp/chatto/issues/1489)) ([ae686d4](https://github.com/chattocorp/chatto/commit/ae686d400ad41b9e6c1a053e59789733bd8bd910))
* **frontend:** enforce server description limit ([#1587](https://github.com/chattocorp/chatto/issues/1587)) ([17693d7](https://github.com/chattocorp/chatto/commit/17693d743932631b76210bc5d3dc3d3beee5e56e))
* **frontend:** expire empty attachment tombstones ([#1952](https://github.com/chattocorp/chatto/issues/1952)) ([410492c](https://github.com/chattocorp/chatto/commit/410492cf176f97eb027226549e5bf1a1f31cb648))
* **frontend:** hide context-free tombstones immediately ([#2142](https://github.com/chattocorp/chatto/issues/2142)) ([54aa45e](https://github.com/chattocorp/chatto/commit/54aa45e65cfe238880464172d5f822782349ef6d))
* **frontend:** hide deleted users from membership events ([#1504](https://github.com/chattocorp/chatto/issues/1504)) ([65fa9a0](https://github.com/chattocorp/chatto/commit/65fa9a0ecb9b528d768e6bf4626ea1029440bc8f))
* **frontend:** hydrate desktop server statuses on startup ([#2029](https://github.com/chattocorp/chatto/issues/2029)) ([ef7375f](https://github.com/chattocorp/chatto/commit/ef7375fd43d24d86f7f84e28c5df0463a95e65c6))
* **frontend:** improve thread badge interaction ([#1935](https://github.com/chattocorp/chatto/issues/1935)) ([3b9d4c7](https://github.com/chattocorp/chatto/commit/3b9d4c70d78f42eb343b13b401af17972ef918d8))
* **frontend:** keep emoji picker open on new messages ([#1603](https://github.com/chattocorp/chatto/issues/1603)) ([2042625](https://github.com/chattocorp/chatto/commit/2042625262973303ea0e8574b60aa166bcff4a9d))
* **frontend:** keep mobile bottom sheets open on input focus ([#1750](https://github.com/chattocorp/chatto/issues/1750)) ([54b8b57](https://github.com/chattocorp/chatto/commit/54b8b57acb522166bbf0daef19b746584d56210e))
* **frontend:** keep permission matrices stable during refresh ([#2178](https://github.com/chattocorp/chatto/issues/2178)) ([2329ead](https://github.com/chattocorp/chatto/commit/2329ead2c9e53725419b5b820f0397e64d48943a))
* **frontend:** keep realtime active while looking offline ([#1606](https://github.com/chattocorp/chatto/issues/1606)) ([7cbbd9b](https://github.com/chattocorp/chatto/commit/7cbbd9bc8bdec0b6db5606d5bb58917374a12878))
* **frontend:** keep remote sessions live without origin auth ([#1877](https://github.com/chattocorp/chatto/issues/1877)) ([55aed52](https://github.com/chattocorp/chatto/commit/55aed52831e54fda3903dcaf09ff7f4c23106de8))
* **frontend:** keep video close button within iOS safe area ([#1618](https://github.com/chattocorp/chatto/issues/1618)) ([cf4210a](https://github.com/chattocorp/chatto/commit/cf4210a1df577c6e4b99b83c84006e972b09f3bc))
* **frontend:** lazily load room attachments ([#1615](https://github.com/chattocorp/chatto/issues/1615)) ([6a5b7e0](https://github.com/chattocorp/chatto/commit/6a5b7e07e4d715c2894e2b20a913df0dbc3ca576))
* **frontend:** log auth/session degradation events with console.warn ([#2093](https://github.com/chattocorp/chatto/issues/2093)) ([811e624](https://github.com/chattocorp/chatto/commit/811e6243451378bb6aeae42d6032360bd2bb12b9))
* **frontend:** move account settings to top ([#2125](https://github.com/chattocorp/chatto/issues/2125)) ([02fa2b8](https://github.com/chattocorp/chatto/commit/02fa2b8823c0ccca47c1d36bbb2e8a27c9be4182))
* **frontend:** parse pasted markdown in composer ([#1714](https://github.com/chattocorp/chatto/issues/1714)) ([c571f70](https://github.com/chattocorp/chatto/commit/c571f70ac9f7b82da041d8fc9162aaa296e9e82e))
* **frontend:** preserve composer drag selections ([#2053](https://github.com/chattocorp/chatto/issues/2053)) ([ededb95](https://github.com/chattocorp/chatto/commit/ededb95120aad270b15d38e3181a70a25c5b217c))
* **frontend:** preserve expanded timeline groups ([#1483](https://github.com/chattocorp/chatto/issues/1483)) ([599696e](https://github.com/chattocorp/chatto/commit/599696e3358421eb70e67f87b000b641caafa082))
* **frontend:** preserve inline ordered-list content ([#2010](https://github.com/chattocorp/chatto/issues/2010)) ([27980b2](https://github.com/chattocorp/chatto/commit/27980b2759a3b428dbdc5fad62d79b8b51fa1603))
* **frontend:** preserve pasted Markdown links ([#1639](https://github.com/chattocorp/chatto/issues/1639)) ([40a496b](https://github.com/chattocorp/chatto/commit/40a496bd5389175dad619ce7fcb30805b0050ddd))
* **frontend:** preserve pasted message line breaks ([#1609](https://github.com/chattocorp/chatto/issues/1609)) ([07eb5c2](https://github.com/chattocorp/chatto/commit/07eb5c2a4f581ce406d0e22646afacc730c39f3d))
* **frontend:** preserve unusual video aspect ratios ([#1521](https://github.com/chattocorp/chatto/issues/1521)) ([106ab5f](https://github.com/chattocorp/chatto/commit/106ab5f783002eef435cb1ed67b2b052ee65e184))
* **frontend:** preserve video playback on reactions ([#1622](https://github.com/chattocorp/chatto/issues/1622)) ([2b75984](https://github.com/chattocorp/chatto/commit/2b759847d6648eaf8bd9300e546dd9435ac4ba02))
* **frontend:** prevent premature attachment URL refreshes ([#1920](https://github.com/chattocorp/chatto/issues/1920)) ([034eb49](https://github.com/chattocorp/chatto/commit/034eb498eb3db8fbd6e9bdaa187647e7568ee7f6))
* **frontend:** proxy OAuth token endpoint in development ([#2101](https://github.com/chattocorp/chatto/issues/2101)) ([38da936](https://github.com/chattocorp/chatto/commit/38da9363c40a73df52f7e963a7fe25f4b820cccb))
* **frontend:** reduce production build memory ([#1912](https://github.com/chattocorp/chatto/issues/1912)) ([1f108cf](https://github.com/chattocorp/chatto/commit/1f108cf04da08dd288942ff603fb81d1efcc0786))
* **frontend:** restore branded PWA install icons ([#1478](https://github.com/chattocorp/chatto/issues/1478)) ([5c915b1](https://github.com/chattocorp/chatto/commit/5c915b16667962df0d23248043f7f94e4d05e62e))
* **frontend:** restore neutral server selection ([#2170](https://github.com/chattocorp/chatto/issues/2170)) ([769b3de](https://github.com/chattocorp/chatto/commit/769b3dedf00327f6a87f23af9cf924b5219a15b4))
* **frontend:** restore POST server discovery ([#1528](https://github.com/chattocorp/chatto/issues/1528)) ([8534ba7](https://github.com/chattocorp/chatto/commit/8534ba78b027e73e26918741d6ea9377a9d17c8d))
* **frontend:** restore subtle design colors ([#1545](https://github.com/chattocorp/chatto/issues/1545)) ([659be37](https://github.com/chattocorp/chatto/commit/659be3751115a8bc654ce3dbdd3fc70714950f4c))
* **frontend:** scope thread click-outside dismissal ([#1595](https://github.com/chattocorp/chatto/issues/1595)) ([ae5b9f9](https://github.com/chattocorp/chatto/commit/ae5b9f99be838d0fe1afb0d9db8eb6661ce3546a))
* **frontend:** separate member sidebar scrolling ([#2177](https://github.com/chattocorp/chatto/issues/2177)) ([c41c124](https://github.com/chattocorp/chatto/commit/c41c124151d7d6b08a39f22efc3d7c8ab46b11aa))
* **frontend:** shorten server navigation heading ([#2148](https://github.com/chattocorp/chatto/issues/2148)) ([f57af58](https://github.com/chattocorp/chatto/commit/f57af58c4a9b921a9fef470af264b3752970aca5))
* **frontend:** show pending permission updates ([#1540](https://github.com/chattocorp/chatto/issues/1540)) ([a33a25e](https://github.com/chattocorp/chatto/commit/a33a25e7517df84d3711bbe10e053f670f66a5c7))
* **frontend:** simplify copy ID affordance ([#2088](https://github.com/chattocorp/chatto/issues/2088)) ([8d1ba0c](https://github.com/chattocorp/chatto/commit/8d1ba0c1a8cce28c3118af53d73f7ce9e966d7dd))
* **frontend:** size ordered list markers adaptively ([#1911](https://github.com/chattocorp/chatto/issues/1911)) ([862f49c](https://github.com/chattocorp/chatto/commit/862f49cb427f828eff37d86016d85d5c65b501b6))
* **frontend:** stabilise composer word wrapping ([#1636](https://github.com/chattocorp/chatto/issues/1636)) ([0e7dde0](https://github.com/chattocorp/chatto/commit/0e7dde0755be8641f3c4159ebb947edefc384292))
* **frontend:** stabilise sign-in and chat landing states ([#2160](https://github.com/chattocorp/chatto/issues/2160)) ([5e46cd3](https://github.com/chattocorp/chatto/commit/5e46cd318661b91c65e7b65c99e38c022b69aea8))
* **frontend:** support remote-only sessions ([#1530](https://github.com/chattocorp/chatto/issues/1530)) ([e7b90c3](https://github.com/chattocorp/chatto/commit/e7b90c3916263e9334b021389b5931c6122ef861))
* **frontend:** unify admin panel and table surfaces ([#1677](https://github.com/chattocorp/chatto/issues/1677)) ([0289c34](https://github.com/chattocorp/chatto/commit/0289c341cdce41fff73262c847f21be4e5570cfa))
* **frontend:** use origin host for message links ([#1526](https://github.com/chattocorp/chatto/issues/1526)) ([b4b872c](https://github.com/chattocorp/chatto/commit/b4b872c4a705caa020745e158132f13b532cf1b2))
* **frontend:** use server logo for browser icons ([#1506](https://github.com/chattocorp/chatto/issues/1506)) ([63c7fde](https://github.com/chattocorp/chatto/commit/63c7fde45f386370d114dfad70279101b5e35a5c))
* **i18n:** restore regional locale fallback chains ([#1987](https://github.com/chattocorp/chatto/issues/1987)) ([d98ecf7](https://github.com/chattocorp/chatto/commit/d98ecf7b77027e18cfed2a9647d03647376e30b1))
* **media:** stabilize attachment URLs and playback ([#1626](https://github.com/chattocorp/chatto/issues/1626)) ([8b8c63b](https://github.com/chattocorp/chatto/commit/8b8c63b9663dd9fbf9d9232cea14158226a53ad6))
* **notifications:** avoid authorization fence events ([#2115](https://github.com/chattocorp/chatto/issues/2115)) ([003f54a](https://github.com/chattocorp/chatto/commit/003f54acfe8075014db83cbb11059ae26066aeae))
* **presence:** handle constant wrong-sequence errors ([#1511](https://github.com/chattocorp/chatto/issues/1511)) ([07a87f1](https://github.com/chattocorp/chatto/commit/07a87f1d0467669a288bbeb835da10f888582147))
* **push:** prevent authenticated SSRF ([#2040](https://github.com/chattocorp/chatto/issues/2040)) ([b6aa43c](https://github.com/chattocorp/chatto/commit/b6aa43c8f5143dec9800700bce5773cc6a80e2ac))
* **push:** prioritize visible notifications ([#1928](https://github.com/chattocorp/chatto/issues/1928)) ([548aca2](https://github.com/chattocorp/chatto/commit/548aca2dacf4c3d2ecf4a6c450942462b95b5e2b))
* **pwa:** restore DM-specific dock badges ([#1631](https://github.com/chattocorp/chatto/issues/1631)) ([df96e94](https://github.com/chattocorp/chatto/commit/df96e949133d4e2c25fa9287935d054700b91eaf))
* **pwa:** simplify app badge synchronization ([#1616](https://github.com/chattocorp/chatto/issues/1616)) ([ac17b36](https://github.com/chattocorp/chatto/commit/ac17b3678cb385294520fc8b5daa8d4e3beb7219))
* **pwa:** use server name for installed app ([#1542](https://github.com/chattocorp/chatto/issues/1542)) ([c574036](https://github.com/chattocorp/chatto/commit/c574036baeea54488483d279c5ca00db74c817f5))
* **reactions:** cap reactions per user and message ([#1919](https://github.com/chattocorp/chatto/issues/1919)) ([79be46c](https://github.com/chattocorp/chatto/commit/79be46cb4f116b807066bf443a83e114d110be47))
* **realtime:** avoid duplicate viewer state on reset ([#1959](https://github.com/chattocorp/chatto/issues/1959)) ([373256f](https://github.com/chattocorp/chatto/commit/373256f97d0261df442c51d7725dceed8838adf6))
* **realtime:** defer retained timeline hydration ([#2109](https://github.com/chattocorp/chatto/issues/2109)) ([80a7a6b](https://github.com/chattocorp/chatto/commit/80a7a6bbf9370f1522c61cf0c1d50ba53dcab75d))
* **realtime:** recover after deleting processing videos ([#1894](https://github.com/chattocorp/chatto/issues/1894)) ([7db7d9c](https://github.com/chattocorp/chatto/commit/7db7d9c345647a9665df1877a3cb6bd77a601292))
* **realtime:** refresh active calls after room access ([#2031](https://github.com/chattocorp/chatto/issues/2031)) ([1c0ae04](https://github.com/chattocorp/chatto/commit/1c0ae045212a180018cb89e3311ab230b7763bcf))
* **release:** develop prereleases on main ([#1419](https://github.com/chattocorp/chatto/issues/1419)) ([a33d440](https://github.com/chattocorp/chatto/commit/a33d440f88f24d44fa6657f24ba09de48e89e857))
* **release:** make next prerelease alpha ([#1596](https://github.com/chattocorp/chatto/issues/1596)) ([5ad172b](https://github.com/chattocorp/chatto/commit/5ad172bd53fdc09dfb6088f525fcb6b2b84bc916))
* **security:** restrict public server assets ([#1499](https://github.com/chattocorp/chatto/issues/1499)) ([2ada7d2](https://github.com/chattocorp/chatto/commit/2ada7d26669ad88f1aea5962ee84f91f9ea789da))
* **threads:** reject echoes as thread roots ([#1757](https://github.com/chattocorp/chatto/issues/1757)) ([e81f585](https://github.com/chattocorp/chatto/commit/e81f585f147eaeafaf8a3b226e28c2599c3bdb2c))
* **video:** harden HLS playback and transcoding ([#1694](https://github.com/chattocorp/chatto/issues/1694)) ([3646898](https://github.com/chattocorp/chatto/commit/3646898405a9acd35b9abbf49f7d07e1b5c1fa7a))
* **workers:** harden durable recovery ([#1978](https://github.com/chattocorp/chatto/issues/1978)) ([c1c6f3e](https://github.com/chattocorp/chatto/commit/c1c6f3e35510da1e19cfdfcc30ed060494600f05))


### Performance Improvements

* **api:** accelerate first-load viewer state ([#1756](https://github.com/chattocorp/chatto/issues/1756)) ([4773539](https://github.com/chattocorp/chatto/commit/4773539abe870509fdf650b474c783c04c9ef243))
* **build:** shrink bundled Chatto binary ([#1890](https://github.com/chattocorp/chatto/issues/1890)) ([021254c](https://github.com/chattocorp/chatto/commit/021254c4b02df32bc1b3584d3d38688b8e1a3926))
* **connect:** reuse DEKs across request hydration ([#1554](https://github.com/chattocorp/chatto/issues/1554)) ([b5a8249](https://github.com/chattocorp/chatto/commit/b5a8249aa798cecd8c7ae106a9a7f2cb7a74b618))
* **core:** accelerate cold projection replay ([#1717](https://github.com/chattocorp/chatto/issues/1717)) ([437521d](https://github.com/chattocorp/chatto/commit/437521d459b70aa5e38a57db0187200938b2c4c9))
* **core:** defer server member detail hydration ([#1977](https://github.com/chattocorp/chatto/issues/1977)) ([41d2a6d](https://github.com/chattocorp/chatto/commit/41d2a6dadc6997ca720e13eb4c01c1ae84c59ac2))
* **frontend:** defer custom status editor ([#1829](https://github.com/chattocorp/chatto/issues/1829)) ([d7a589f](https://github.com/chattocorp/chatto/commit/d7a589f1c2d30d7cdc8d5a38060ccc9d6f1ad5b5))
* **frontend:** defer room interaction bundles ([#1834](https://github.com/chattocorp/chatto/issues/1834)) ([04f82ee](https://github.com/chattocorp/chatto/commit/04f82ee5a4a55b3239bb55ffcbbacbaebb0cc916))
* **frontend:** preserve lazy chunk boundaries ([#1820](https://github.com/chattocorp/chatto/issues/1820)) ([cc58883](https://github.com/chattocorp/chatto/commit/cc588833d591ddce1b66b165219169a0fbd89e8e))
* **frontend:** reduce initial route bundles ([#1825](https://github.com/chattocorp/chatto/issues/1825)) ([936162e](https://github.com/chattocorp/chatto/commit/936162ea602768afb5de7c7eb06b13a029e8aa4e))
* **frontend:** skip dev-only production work ([#1914](https://github.com/chattocorp/chatto/issues/1914)) ([9e874f8](https://github.com/chattocorp/chatto/commit/9e874f845bb498dababb8456f69f66361a9f56a9))
* **frontend:** speed up Paraglide compilation ([#1562](https://github.com/chattocorp/chatto/issues/1562)) ([185f306](https://github.com/chattocorp/chatto/commit/185f3067c83831280a2deb365e8f50d2813cce61))
* **frontend:** stabilise room member presence grouping ([#1903](https://github.com/chattocorp/chatto/issues/1903)) ([3f6eaab](https://github.com/chattocorp/chatto/commit/3f6eaab89af09ce9012389c09401e3a334a1e66e))
* **projections:** compact timeline body state ([#1720](https://github.com/chattocorp/chatto/issues/1720)) ([59b9dd3](https://github.com/chattocorp/chatto/commit/59b9dd3a3c23ba3c037b3090887292b07e9dc430))
* **realtime:** centralize myEvents fanout ([#1513](https://github.com/chattocorp/chatto/issues/1513)) ([1c46f88](https://github.com/chattocorp/chatto/commit/1c46f88b78d465c3b7c594f4c4c98712af96452d))


### Code Refactoring

* **auth:** remove legacy cookie sessions ([#2087](https://github.com/chattocorp/chatto/issues/2087)) ([0e30804](https://github.com/chattocorp/chatto/commit/0e308044fd3e70560225346b9eecce02716cf8fe))
* **authz:** flatten interaction read permission ([#2175](https://github.com/chattocorp/chatto/issues/2175)) ([8f4e86c](https://github.com/chattocorp/chatto/commit/8f4e86c9d5d1ef5a6fea264b9ac4a51d483d9a8f))
* **cli:** remove passphrase argument flags ([#1705](https://github.com/chattocorp/chatto/issues/1705)) ([bb84c8a](https://github.com/chattocorp/chatto/commit/bb84c8ac8d6ef31fa395c8d7ed6353feaa8f620c))
* **metrics:** retire legacy service inventory ([#1703](https://github.com/chattocorp/chatto/issues/1703)) ([a5c31a7](https://github.com/chattocorp/chatto/commit/a5c31a7260c263592e467e05f519400d4a2ff04f))
* **proto:** relocate live-only user payloads ([#2153](https://github.com/chattocorp/chatto/issues/2153)) ([bb01e27](https://github.com/chattocorp/chatto/commit/bb01e27ffaf944833d6ec1f7769038742cfcd912))
* **proto:** separate internal storage contracts ([#2162](https://github.com/chattocorp/chatto/issues/2162)) ([9e935d4](https://github.com/chattocorp/chatto/commit/9e935d4701bfb2ad4c8205c0cdbe24c5b9842545))

## [0.4.8](https://github.com/chattocorp/chatto/compare/v0.4.7...v0.4.8) (2026-07-12)


### Bug Fixes

* **api:** bound message attachment asset IDs ([#1458](https://github.com/chattocorp/chatto/issues/1458)) ([f22b343](https://github.com/chattocorp/chatto/commit/f22b343a0b8d4dba97e480a10f107ecb30f90e3a))
* **auth:** keep provider redirects credential-free ([#1437](https://github.com/chattocorp/chatto/issues/1437)) ([e9c5ebf](https://github.com/chattocorp/chatto/commit/e9c5ebff450704e96acdebb9cfe19f858ccd497b))
* **auth:** preserve sessions during storage outages ([#1431](https://github.com/chattocorp/chatto/issues/1431)) ([3442f8a](https://github.com/chattocorp/chatto/commit/3442f8aa9f37a21f01c57a7b5c65c7ffa5d833c4))
* **auth:** throttle password reset requests ([#1441](https://github.com/chattocorp/chatto/issues/1441)) ([67dd1dc](https://github.com/chattocorp/chatto/commit/67dd1dc682d813685efe7454780fe86c09083724))
* **backup:** harden archive creation and restore ([#1435](https://github.com/chattocorp/chatto/issues/1435)) ([d07672c](https://github.com/chattocorp/chatto/commit/d07672c23ab6023520d2424ecff17bce374c31ca))
* **cli:** require explicit passphrase sources ([#1451](https://github.com/chattocorp/chatto/issues/1451)) ([6b8b24e](https://github.com/chattocorp/chatto/commit/6b8b24e9679652ddec6d3db93c7d48b93ffd6104))
* **frontend:** embed main build version ([#1463](https://github.com/chattocorp/chatto/issues/1463)) ([21a20a8](https://github.com/chattocorp/chatto/commit/21a20a856e9ff234b6f74b2045246cfff038cc8d))
* **frontend:** label deleted users consistently ([#1452](https://github.com/chattocorp/chatto/issues/1452)) ([8e04235](https://github.com/chattocorp/chatto/commit/8e042350b1c6d91c0df3affc4402b46a330bbfcf))
* **release:** publish sortable main image tags ([030db55](https://github.com/chattocorp/chatto/commit/030db55d60cdd039c30904e460988b6a29cd4485))
* **security:** harden realtime auth and request handling ([#1433](https://github.com/chattocorp/chatto/issues/1433)) ([75c5a24](https://github.com/chattocorp/chatto/commit/75c5a246d4e6b75c7750b99c19e5e3aa4bbbe3bd))
* **security:** require explicit proxy trust ([#1447](https://github.com/chattocorp/chatto/issues/1447)) ([234862f](https://github.com/chattocorp/chatto/commit/234862f412943cc481ed0744ed4c7b4e6473aff0))
* **tls:** secure autocert cache permissions ([#1461](https://github.com/chattocorp/chatto/issues/1461)) ([cef9d29](https://github.com/chattocorp/chatto/commit/cef9d292e722ef6998356419db79a23b4c14c19d))

## [0.4.7](https://github.com/chattocorp/chatto/compare/v0.4.6...v0.4.7) (2026-07-11)


### Bug Fixes

* **frontend:** show server logo before login ([#1416](https://github.com/chattocorp/chatto/issues/1416)) ([577cf17](https://github.com/chattocorp/chatto/commit/577cf179b0a1bb669813a619896c41fb81d24d17))
* **frontend:** stabilize linked-message navigation ([#1421](https://github.com/chattocorp/chatto/issues/1421)) ([721c974](https://github.com/chattocorp/chatto/commit/721c974bb0e02f62c654389588cdd34346f4fca9))
* **frontend:** support file drops in threads ([#1417](https://github.com/chattocorp/chatto/issues/1417)) ([6166dc0](https://github.com/chattocorp/chatto/commit/6166dc0a1537ea1c483a8c330b3ee515fc16235c))
* **release:** add next prerelease channel ([#1414](https://github.com/chattocorp/chatto/issues/1414)) ([6312e39](https://github.com/chattocorp/chatto/commit/6312e392fa1c0f16c74e5dce65166d586a1e76ca))


### Performance Improvements

* **frontend:** speed up large room member loading ([#1423](https://github.com/chattocorp/chatto/issues/1423)) ([6d4ce8a](https://github.com/chattocorp/chatto/commit/6d4ce8a5deaef0af8a34373ceae107359d01554a))

## [0.4.6](https://github.com/chattocorp/chatto/compare/v0.4.5...v0.4.6) (2026-07-11)


### Bug Fixes

* **api:** validate custom status emoji ([#1408](https://github.com/chattocorp/chatto/issues/1408)) ([cb62f72](https://github.com/chattocorp/chatto/commit/cb62f725eeab071b66d61b599eda0b74e154d573))


### Performance Improvements

* **projections:** bound replay idempotency memory ([#1407](https://github.com/chattocorp/chatto/issues/1407)) ([7dd3841](https://github.com/chattocorp/chatto/commit/7dd38411d8f6144f1a7126d13b23440348d09927))
* **projections:** remove redundant string interning ([#1411](https://github.com/chattocorp/chatto/issues/1411)) ([c69ef30](https://github.com/chattocorp/chatto/commit/c69ef30babce67e6b5ec8fe3d00a490bd626c545))

## [0.4.5](https://github.com/chattocorp/chatto/compare/v0.4.4...v0.4.5) (2026-07-11)


### Bug Fixes

* **assets:** recover physical deletion from events ([#1394](https://github.com/chattocorp/chatto/issues/1394)) ([e4d2a85](https://github.com/chattocorp/chatto/commit/e4d2a854ccf7549e423145c7ece0f926fd32d410))
* **core:** remove room leavers from voice calls ([#1373](https://github.com/chattocorp/chatto/issues/1373)) ([e0b1ad7](https://github.com/chattocorp/chatto/commit/e0b1ad7811eaaeaa3e7821268bbcdab8c73465b1))
* **docker:** support read-only root filesystems ([#1403](https://github.com/chattocorp/chatto/issues/1403)) ([76462a4](https://github.com/chattocorp/chatto/commit/76462a48b791c2f2b72ee2b2afe2c79ee13b5ef8))
* **docs:** correct deployment guide redirect ([#1395](https://github.com/chattocorp/chatto/issues/1395)) ([aded4e0](https://github.com/chattocorp/chatto/commit/aded4e093d32dbc94f6fc8046c58ff02aa501497))
* **frontend:** add optimistic room reads ([#1376](https://github.com/chattocorp/chatto/issues/1376)) ([22ffc62](https://github.com/chattocorp/chatto/commit/22ffc624a07f929b35a117690aef0c920b28294e))
* **frontend:** keep signed-out servers navigable ([#1397](https://github.com/chattocorp/chatto/issues/1397)) ([c3e281a](https://github.com/chattocorp/chatto/commit/c3e281a39be93639ced703a2943ddc4402c9bcfe))
* **frontend:** preserve optimistic reads across refresh ([#1393](https://github.com/chattocorp/chatto/issues/1393)) ([b77270d](https://github.com/chattocorp/chatto/commit/b77270d5912594220bd6fc470ae397a529f13b18))
* **frontend:** respect browser region in timestamps ([#1387](https://github.com/chattocorp/chatto/issues/1387)) ([7ca3b92](https://github.com/chattocorp/chatto/commit/7ca3b9284bc09de0723dff84cdd2a664162a8e29))
* **release:** restore Windows builds ([#1405](https://github.com/chattocorp/chatto/issues/1405)) ([f95a668](https://github.com/chattocorp/chatto/commit/f95a6680eb48bab7a382e1d95610c1bd6fde91e0))


### Performance Improvements

* **realtime:** bound WebSocket compression memory ([#1400](https://github.com/chattocorp/chatto/issues/1400)) ([c794e59](https://github.com/chattocorp/chatto/commit/c794e5925f81eca00b89fe9e06aaea98870b466c))
* **realtime:** reduce per-connection memory ([#1389](https://github.com/chattocorp/chatto/issues/1389)) ([963287d](https://github.com/chattocorp/chatto/commit/963287d6b16636e7919f8eecf3bd62e7e56759fa))

## [0.4.4](https://github.com/chattocorp/chatto/compare/v0.4.3...v0.4.4) (2026-07-10)


### Bug Fixes

* **api:** skip invalid followed-thread rooms ([#1366](https://github.com/chattocorp/chatto/issues/1366)) ([90a5918](https://github.com/chattocorp/chatto/commit/90a5918a8de8ce9bd857c34806a18e41f036fdf9))
* **docs:** add per-page social previews ([#1370](https://github.com/chattocorp/chatto/issues/1370)) ([69eab4a](https://github.com/chattocorp/chatto/commit/69eab4a3ff2a0299488299f549b198a973a8d8a9))
* **frontend:** allow any file in attachment picker ([#1364](https://github.com/chattocorp/chatto/issues/1364)) ([5b20e17](https://github.com/chattocorp/chatto/commit/5b20e176fca1248ccf118bd63c8dec5892a4512c))
* **frontend:** compress displayed images ([#1361](https://github.com/chattocorp/chatto/issues/1361)) ([5539816](https://github.com/chattocorp/chatto/commit/553981628cb0a0a40f27f2576abbf289f3f9c5b9))
* **messages:** expire context-free tombstones ([#1365](https://github.com/chattocorp/chatto/issues/1365)) ([98123b5](https://github.com/chattocorp/chatto/commit/98123b520c9a6242dd55bffb6594673192787d62))
* **notifications:** harden delivery and synchronization ([#1363](https://github.com/chattocorp/chatto/issues/1363)) ([b46e012](https://github.com/chattocorp/chatto/commit/b46e012075c7f14d664f60f1a05042e38c782243))
* **notifications:** prevent stale push delivery and badges ([#1368](https://github.com/chattocorp/chatto/issues/1368)) ([3c588aa](https://github.com/chattocorp/chatto/commit/3c588aa4d6cb53237be5519b1aa15fc85e094d1c))
* **pwa:** use server logo for install icons ([#1371](https://github.com/chattocorp/chatto/issues/1371)) ([49b26e9](https://github.com/chattocorp/chatto/commit/49b26e9c4f0efc269bcabbd091e63a630bf6913d))

## [0.4.3](https://github.com/chattocorp/chatto/compare/v0.4.2...v0.4.3) (2026-07-09)


### Bug Fixes

* **api:** tune room member page defaults ([#1354](https://github.com/chattocorp/chatto/issues/1354)) ([07675ee](https://github.com/chattocorp/chatto/commit/07675ee459af03edc184cd67171ebb9f7e105b63))
* backfill sparse room timelines ([#1353](https://github.com/chattocorp/chatto/issues/1353)) ([f8adf6a](https://github.com/chattocorp/chatto/commit/f8adf6a73b9a87510f1a17d0440c124472206686))
* **docs:** add community chat link ([#1344](https://github.com/chattocorp/chatto/issues/1344)) ([10c1eec](https://github.com/chattocorp/chatto/commit/10c1eec5fe03190f18b6ab45a5580a8a814d7ed6))
* **frontend:** add optimistic reactions ([#1349](https://github.com/chattocorp/chatto/issues/1349)) ([beaf180](https://github.com/chattocorp/chatto/commit/beaf180e79b6e1248879ec69e495568b24f18173))
* **frontend:** harden login redirect path validation ([#1340](https://github.com/chattocorp/chatto/issues/1340)) ([01db3e3](https://github.com/chattocorp/chatto/commit/01db3e3032950bf95793df64707ee655bcd9d99b))
* **frontend:** stabilize unread and resume refresh ([#1346](https://github.com/chattocorp/chatto/issues/1346)) ([d2f185a](https://github.com/chattocorp/chatto/commit/d2f185afca39be076ec6beda9493d1008319c53e))
* **frontend:** use opaque PWA install icons ([#1352](https://github.com/chattocorp/chatto/issues/1352)) ([afd025c](https://github.com/chattocorp/chatto/commit/afd025c2827a6b3914f6f6a737a340341fc3ea33))
* **metrics:** track realtime websocket connections ([#1356](https://github.com/chattocorp/chatto/issues/1356)) ([fd3ca58](https://github.com/chattocorp/chatto/commit/fd3ca582cee03cf4c3c9f753d1a14967fe0d6b1d))
* **pwa:** stop preserving push badge hints ([#1343](https://github.com/chattocorp/chatto/issues/1343)) ([d9e50a3](https://github.com/chattocorp/chatto/commit/d9e50a3912fff800b1ed71c3a42a91d3e85dbd52))
* **realtime:** align heartbeat cadence with client stall detection ([#1342](https://github.com/chattocorp/chatto/issues/1342)) ([c0e0d23](https://github.com/chattocorp/chatto/commit/c0e0d236ea4d29de7e75b63502323fe7be3ae967))

## [0.4.2](https://github.com/chattocorp/chatto/compare/v0.4.1...v0.4.2) (2026-07-07)


### Bug Fixes

* **api:** tolerate invalid presence user ids ([#1336](https://github.com/chattocorp/chatto/issues/1336)) ([76f1bef](https://github.com/chattocorp/chatto/commit/76f1befee805f7db0a5a2810f6f9fa160e273e35))
* **frontend:** render inline room join screen ([#1335](https://github.com/chattocorp/chatto/issues/1335)) ([af7c831](https://github.com/chattocorp/chatto/commit/af7c831d15c1b0d641ea0e367add127d629bdd92))
* **frontend:** separate input mode from viewport size ([#1339](https://github.com/chattocorp/chatto/issues/1339)) ([217da21](https://github.com/chattocorp/chatto/commit/217da2157820d2ce45e1912e0cbfec2b152c7fca))
* **frontend:** unify user card presence source ([#1334](https://github.com/chattocorp/chatto/issues/1334)) ([dde9b14](https://github.com/chattocorp/chatto/commit/dde9b1435bcbd31958de568e6bbf9a861a77f0d0))
* **push:** add declarative web push payloads ([#1338](https://github.com/chattocorp/chatto/issues/1338)) ([ede325d](https://github.com/chattocorp/chatto/commit/ede325dfdc1c29683fb0739bb7bade9659136eb0))

## [0.4.1](https://github.com/chattocorp/chatto/compare/v0.4.0...v0.4.1) (2026-07-07)


### Bug Fixes

* **api:** log internal connect errors ([#1329](https://github.com/chattocorp/chatto/issues/1329)) ([c292bac](https://github.com/chattocorp/chatto/commit/c292bac00bfefbb4ba0cdbc0f1686b1a377e380d))
* **frontend:** adopt menu shell for toasts ([#1323](https://github.com/chattocorp/chatto/issues/1323)) ([4982463](https://github.com/chattocorp/chatto/commit/4982463f6c396f7ae25ae83985f71453fb74a94d))
* **frontend:** align message footer row spacing ([#1331](https://github.com/chattocorp/chatto/issues/1331)) ([02841fe](https://github.com/chattocorp/chatto/commit/02841fe39c93b3821eaa32f37e67ea32de799577))
* **frontend:** hydrate room lifecycle event actors ([#1319](https://github.com/chattocorp/chatto/issues/1319)) ([a9abe8c](https://github.com/chattocorp/chatto/commit/a9abe8c03fd1e4e539d0870d56ca7569fbe4d2b5))
* **frontend:** improve chat link navigation ([#1333](https://github.com/chattocorp/chatto/issues/1333)) ([a88133f](https://github.com/chattocorp/chatto/commit/a88133fd05199ef23597f0b2258e2f1fb3dc06ec))
* **frontend:** improve reaction user popovers ([#1328](https://github.com/chattocorp/chatto/issues/1328)) ([2d1af04](https://github.com/chattocorp/chatto/commit/2d1af046e3fdc910762ea73bfb4ce78223865a9c))
* **frontend:** refresh recent emoji quick reactions ([#1327](https://github.com/chattocorp/chatto/issues/1327)) ([fab2ae4](https://github.com/chattocorp/chatto/commit/fab2ae44c5b57ca1a0dc8703e46eb244a27e2b83))
* **frontend:** simplify push notification click routing ([#1322](https://github.com/chattocorp/chatto/issues/1322)) ([21bff8d](https://github.com/chattocorp/chatto/commit/21bff8dac0996e087dbc0aeb4d23d21925b1fea2))
* **frontend:** simplify room resume catch-up ([#1332](https://github.com/chattocorp/chatto/issues/1332)) ([b7d32ce](https://github.com/chattocorp/chatto/commit/b7d32ce1c7756839f0f90d15d018f0c1023c0455))
* **frontend:** stabilize mobile sidebar gestures ([#1324](https://github.com/chattocorp/chatto/issues/1324)) ([1cbd3c5](https://github.com/chattocorp/chatto/commit/1cbd3c5f8b0adcc7b824c1023123c2498e81a225))
* **read-state:** reduce no-op read signals ([#1330](https://github.com/chattocorp/chatto/issues/1330)) ([244e4c8](https://github.com/chattocorp/chatto/commit/244e4c82851de9dc9ab9bc7a2b982e840840f631))
* **server:** reduce routine info logs ([#1325](https://github.com/chattocorp/chatto/issues/1325)) ([8849fd7](https://github.com/chattocorp/chatto/commit/8849fd7cd018b6a9112e920ef271b79fd0674629))

## [0.4.0](https://github.com/chattocorp/chatto/compare/v0.3.8...v0.4.0) (2026-07-06)


### ⚠ BREAKING CHANGES

* **api:** consolidate ConnectRPC surface ([#1306](https://github.com/chattocorp/chatto/issues/1306))
* **api:** clean up server assets calls and includes ([#1303](https://github.com/chattocorp/chatto/issues/1303))
* **api:** consolidate shared api shapes ([#1302](https://github.com/chattocorp/chatto/issues/1302))
* **api:** consolidate shared public API types ([#1299](https://github.com/chattocorp/chatto/issues/1299))
* **api:** consolidate public ConnectRPC API ([#1295](https://github.com/chattocorp/chatto/issues/1295))
* **api:** polish ConnectRPC API for 0.4.0 ([#1224](https://github.com/chattocorp/chatto/issues/1224))
* **operator:** add socket-backed operator user administration ([#1164](https://github.com/chattocorp/chatto/issues/1164))
* **api:** reshape server profile responses ([#1185](https://github.com/chattocorp/chatto/issues/1185))
* **api:** split ConnectRPC packages ([#1179](https://github.com/chattocorp/chatto/issues/1179))
* **api:** replace GraphQL with ConnectRPC ([#1166](https://github.com/chattocorp/chatto/issues/1166))
* **api:** use optional timeline presence fields ([#1110](https://github.com/chattocorp/chatto/issues/1110))

### Features

* add universal rooms ([#1046](https://github.com/chattocorp/chatto/issues/1046)) ([0b8c5cb](https://github.com/chattocorp/chatto/commit/0b8c5cb839876416a8262260ddc6a051ee0c94ba))
* **admin:** filter event log ([#1056](https://github.com/chattocorp/chatto/issues/1056)) ([d8bd280](https://github.com/chattocorp/chatto/commit/d8bd28076112e4e2a1488190cb29e9bf0acbc5cc))
* **api:** add ConnectRPC asset uploads ([#1249](https://github.com/chattocorp/chatto/issues/1249)) ([f97f1d0](https://github.com/chattocorp/chatto/commit/f97f1d097ba887279b228bcb0dd243cfd16f320b))
* **api:** add ConnectRPC DM start ([#1157](https://github.com/chattocorp/chatto/issues/1157)) ([c46ef79](https://github.com/chattocorp/chatto/commit/c46ef79ce782fad2f9cd26cb4db42fd7ae581a30))
* **api:** add ConnectRPC public API PoC ([#1067](https://github.com/chattocorp/chatto/issues/1067)) ([7aeb8f7](https://github.com/chattocorp/chatto/commit/7aeb8f7fd629da040d2e916600215fe3d02d0f26))
* **api:** add ConnectRPC reflection ([#1182](https://github.com/chattocorp/chatto/issues/1182)) ([a93324c](https://github.com/chattocorp/chatto/commit/a93324cf91e21cfab6eb7057f9b35e3545f3cf4c))
* **api:** add ConnectRPC room timeline PoC ([#1074](https://github.com/chattocorp/chatto/issues/1074)) ([920fcaa](https://github.com/chattocorp/chatto/commit/920fcaa26ca577ada529e2e1ef19d041d5baa47f))
* **api:** add protobuf realtime websocket ([#1158](https://github.com/chattocorp/chatto/issues/1158)) ([9e8e34c](https://github.com/chattocorp/chatto/commit/9e8e34cdc778be86007d0f6596468b445cfa4a0e))
* **api:** add resource batch reads ([#1232](https://github.com/chattocorp/chatto/issues/1232)) ([8a04ae0](https://github.com/chattocorp/chatto/commit/8a04ae0fa619efc180ff364098f986859f33e041))
* **api:** clean up ConnectRPC surface ([#1171](https://github.com/chattocorp/chatto/issues/1171)) ([03c42af](https://github.com/chattocorp/chatto/commit/03c42af51837bcd999bb3c34989ba706e2d291c5))
* **api:** clean up ConnectRPC surface ([#1178](https://github.com/chattocorp/chatto/issues/1178)) ([b1b6e28](https://github.com/chattocorp/chatto/commit/b1b6e28a818d3f878c0674bd741292d1e33f680e))
* **api:** clean up server assets calls and includes ([#1303](https://github.com/chattocorp/chatto/issues/1303)) ([e960def](https://github.com/chattocorp/chatto/commit/e960defc9c3a1cc77ae1958a8c98d9cc54919c25))
* **api:** consolidate membership services ([#1293](https://github.com/chattocorp/chatto/issues/1293)) ([7ed268c](https://github.com/chattocorp/chatto/commit/7ed268c71443c75201a1d26036f318f8df6f6e05))
* **api:** consolidate shared api shapes ([#1302](https://github.com/chattocorp/chatto/issues/1302)) ([4429009](https://github.com/chattocorp/chatto/commit/4429009ba3dd1b4ce0800b928c55fb8eaa308376))
* **api:** consolidate shared public API types ([#1299](https://github.com/chattocorp/chatto/issues/1299)) ([1ec2015](https://github.com/chattocorp/chatto/commit/1ec201551881142d8d5498902d0ff192e7b8bf7e))
* **api:** extract generated TypeScript clients ([#1183](https://github.com/chattocorp/chatto/issues/1183)) ([3480cda](https://github.com/chattocorp/chatto/commit/3480cdab949940d614160897134129693f14e782))
* **api:** extract TypeScript API client ([#1184](https://github.com/chattocorp/chatto/issues/1184)) ([b38b9a5](https://github.com/chattocorp/chatto/commit/b38b9a522cd48b5673109d09007b7d04709b251e))
* **api:** migrate reactions to ConnectRPC ([#1128](https://github.com/chattocorp/chatto/issues/1128)) ([161f51c](https://github.com/chattocorp/chatto/commit/161f51ccb4cc0cd3b1b098d1b5aa41c3f4405c8d))
* **api:** polish ConnectRPC API for 0.4.0 ([#1224](https://github.com/chattocorp/chatto/issues/1224)) ([06f4361](https://github.com/chattocorp/chatto/commit/06f4361d05e27587839e31b128e38b3ee011c743))
* **api:** port message posting to ConnectRPC ([#1093](https://github.com/chattocorp/chatto/issues/1093)) ([011018b](https://github.com/chattocorp/chatto/commit/011018bab165ba29e310f2e527a6dae9648899e2))
* **api:** port read state and thread follow to ConnectRPC ([#1087](https://github.com/chattocorp/chatto/issues/1087)) ([f2128d6](https://github.com/chattocorp/chatto/commit/f2128d60d6d1706217f06566102788900619e053))
* **api:** replace GraphQL with ConnectRPC ([#1166](https://github.com/chattocorp/chatto/issues/1166)) ([3dd3fa6](https://github.com/chattocorp/chatto/commit/3dd3fa686fc3c89912dcdf02475578389608f627))
* **api:** reshape server profile responses ([#1185](https://github.com/chattocorp/chatto/issues/1185)) ([96bde6e](https://github.com/chattocorp/chatto/commit/96bde6eb3d0ea9b134e7191e41b16fdc07d3bee1))
* **api:** split ConnectRPC packages ([#1179](https://github.com/chattocorp/chatto/issues/1179)) ([6ec286a](https://github.com/chattocorp/chatto/commit/6ec286a469377b5ebe338167cb0244bbc4a9b9d2))
* **api:** use optional timeline presence fields ([#1110](https://github.com/chattocorp/chatto/issues/1110)) ([5c1406f](https://github.com/chattocorp/chatto/commit/5c1406f0a28502be869964c87561c0e107c81446))
* **auth:** add SSO account creation and linking ([#1167](https://github.com/chattocorp/chatto/issues/1167)) ([61723e9](https://github.com/chattocorp/chatto/commit/61723e9e3e6c6f8802558c8a11acab31444c7efb))
* **auth:** type runtime credentials ([#1195](https://github.com/chattocorp/chatto/issues/1195)) ([5f0ebe4](https://github.com/chattocorp/chatto/commit/5f0ebe4264d4f4539ce85f4d8c3d1a6a779a9702))
* **config:** configure SMTP TLS verification ([#1159](https://github.com/chattocorp/chatto/issues/1159)) ([1f5c8b0](https://github.com/chattocorp/chatto/commit/1f5c8b09d2f4c13d0c13825c38e2bb5c4807beeb))
* **connectrpc:** add message management API ([#1146](https://github.com/chattocorp/chatto/issues/1146)) ([c07b049](https://github.com/chattocorp/chatto/commit/c07b0497ab09ae970895809edb5b31fd79c5e093))
* **connectrpc:** add room directory service ([#1138](https://github.com/chattocorp/chatto/issues/1138)) ([c1f13cf](https://github.com/chattocorp/chatto/commit/c1f13cfb4d0dc9cacb019c430db4f8494026ed02))
* **connectrpc:** add room lifecycle service ([#1134](https://github.com/chattocorp/chatto/issues/1134)) ([3f2b3a9](https://github.com/chattocorp/chatto/commit/3f2b3a922f97c4f99f20913e4e4d4a944bb79704))
* **connectrpc:** port thread history reads ([#1083](https://github.com/chattocorp/chatto/issues/1083)) ([4b81b4d](https://github.com/chattocorp/chatto/commit/4b81b4dbf78e879cdf2b10060f3777f6d2071dc3))
* **core:** persist link preview assets via storage backend ([#1060](https://github.com/chattocorp/chatto/issues/1060)) ([005deb1](https://github.com/chattocorp/chatto/commit/005deb1365f1899176cca57f91db8265cf7da009))
* **core:** store thread follows in EVT ([#1233](https://github.com/chattocorp/chatto/issues/1233)) ([01a2bb3](https://github.com/chattocorp/chatto/commit/01a2bb3d629b83dd30431afcb17e3746a4848d33))
* **dev:** add Mailpit to mise dev ([#1238](https://github.com/chattocorp/chatto/issues/1238)) ([0d07f7e](https://github.com/chattocorp/chatto/commit/0d07f7e8d9540de1d36cf56388f151bd94cb3f2b))
* **docs:** add release notes pages ([#1180](https://github.com/chattocorp/chatto/issues/1180)) ([6418471](https://github.com/chattocorp/chatto/commit/641847194e8d02cd86e8e9827b756a8cec109d56))
* **exporter:** add deployment-wide prometheus exporter ([#1059](https://github.com/chattocorp/chatto/issues/1059)) ([5aa29c7](https://github.com/chattocorp/chatto/commit/5aa29c747babe5b4dacc12a9a63eef57bcf36ec8))
* **frontend:** add multi-image attachment gallery ([#1241](https://github.com/chattocorp/chatto/issues/1241)) ([d8338c5](https://github.com/chattocorp/chatto/commit/d8338c517ef71069a08db44f402b949458ea6e92))
* **frontend:** add Paraglide-based client-shell i18n ([#1077](https://github.com/chattocorp/chatto/issues/1077)) ([1a4ab07](https://github.com/chattocorp/chatto/commit/1a4ab07211482af1236b3921607fd2deb8746f4f))
* **frontend:** add Trusted Types markdown policy ([#1307](https://github.com/chattocorp/chatto/issues/1307)) ([47b9060](https://github.com/chattocorp/chatto/commit/47b9060a0ba84df49464a564225a88914393d2e3))
* **frontend:** consolidate frontend design system ([#1053](https://github.com/chattocorp/chatto/issues/1053)) ([7fc39ab](https://github.com/chattocorp/chatto/commit/7fc39ab6aebdba74bd8eef56ba05323bf60ad901))
* **frontend:** improve admin member details ([#1057](https://github.com/chattocorp/chatto/issues/1057)) ([8c8ccce](https://github.com/chattocorp/chatto/commit/8c8cccee5335bf2d10948414a65b2d75a547c30f))
* **frontend:** maximize call pane ([#1240](https://github.com/chattocorp/chatto/issues/1240)) ([7aaa34a](https://github.com/chattocorp/chatto/commit/7aaa34ad4abb9d27cb558b10a8c8944a80240de7))
* **frontend:** move UI strings into i18n catalogs ([#1084](https://github.com/chattocorp/chatto/issues/1084)) ([d310382](https://github.com/chattocorp/chatto/commit/d310382e0795007da388e0514ac7d2056e961898))
* **frontend:** refresh admin system dashboard ([#1160](https://github.com/chattocorp/chatto/issues/1160)) ([5c54899](https://github.com/chattocorp/chatto/commit/5c54899f1eb676cff77ca3707b9e98eb36b639c6))
* **frontend:** refresh toast styling ([#1260](https://github.com/chattocorp/chatto/issues/1260)) ([1b728e5](https://github.com/chattocorp/chatto/commit/1b728e511e6b6310d10d59bf7c6085d4c70710d0))
* **frontend:** send typing indicators with ConnectRPC ([#1155](https://github.com/chattocorp/chatto/issues/1155)) ([1a131ee](https://github.com/chattocorp/chatto/commit/1a131eea08bb32a89462bbd0c010617cc2fdaedb))
* **frontend:** show call participants in room sidebar ([#1036](https://github.com/chattocorp/chatto/issues/1036)) ([8cd0858](https://github.com/chattocorp/chatto/commit/8cd085877d44633aa54578abf2d50a62942c0085))
* **frontend:** show reaction names in popups ([#1044](https://github.com/chattocorp/chatto/issues/1044)) ([e141b74](https://github.com/chattocorp/chatto/commit/e141b7441ca7d8d62252f2a9376ca3f2a768ea9d))
* **frontend:** show room descriptions in header ([#1037](https://github.com/chattocorp/chatto/issues/1037)) ([44f9c67](https://github.com/chattocorp/chatto/commit/44f9c67c979535584c12838ccc46eaf40a879d6c))
* **frontend:** use ConnectRPC for message writes ([#1153](https://github.com/chattocorp/chatto/issues/1153)) ([4b34f34](https://github.com/chattocorp/chatto/commit/4b34f341f4e96adb87d775c5ea2fc0ae04e12aee))
* **frontend:** use ConnectRPC for room commands ([#1150](https://github.com/chattocorp/chatto/issues/1150)) ([bfff68e](https://github.com/chattocorp/chatto/commit/bfff68e8d48a2adbd512be249e9482c467b03a88))
* **operator:** add socket-backed operator user administration ([#1164](https://github.com/chattocorp/chatto/issues/1164)) ([6209795](https://github.com/chattocorp/chatto/commit/6209795767fa38e2031bfb77e61b3bcb034a4b77))
* **presence:** add user-controlled presence modes ([#1095](https://github.com/chattocorp/chatto/issues/1095)) ([9e8f696](https://github.com/chattocorp/chatto/commit/9e8f696df7dc2489c639479f01eb7269ba13a922))
* **profile:** add custom user statuses ([#1081](https://github.com/chattocorp/chatto/issues/1081)) ([1d1d7d2](https://github.com/chattocorp/chatto/commit/1d1d7d214a28b9c9eb38c50522e44b943d7e5cb5))


### Bug Fixes

* **api:** address 0.4.0 surface review findings ([#1228](https://github.com/chattocorp/chatto/issues/1228)) ([bd054ff](https://github.com/chattocorp/chatto/commit/bd054ff0102c3065781064726c1d128f3980700e))
* **api:** align ConnectRPC permission exposure ([#1246](https://github.com/chattocorp/chatto/issues/1246)) ([cf2eca7](https://github.com/chattocorp/chatto/commit/cf2eca7877b10406f517e64f542fd56d1e73594e))
* **api:** centralize Connect room RBAC in core ([#1149](https://github.com/chattocorp/chatto/issues/1149)) ([8ba5b0c](https://github.com/chattocorp/chatto/commit/8ba5b0c2a3854f1ca7f18084a3225661a5e3d205))
* **api:** close ConnectRPC RBAC gaps ([#1207](https://github.com/chattocorp/chatto/issues/1207)) ([da0b129](https://github.com/chattocorp/chatto/commit/da0b1298db513bdc7a95319535039a01a04010e7))
* **api:** include user status in generated docs ([#1092](https://github.com/chattocorp/chatto/issues/1092)) ([52521fa](https://github.com/chattocorp/chatto/commit/52521fa5eeff94d9bebffabb010a6eb4b5e9de78))
* **api:** make ConnectRPC plumbing idiomatic ([#1123](https://github.com/chattocorp/chatto/issues/1123)) ([338f573](https://github.com/chattocorp/chatto/commit/338f57315cf611518ff4570434ee7faae1ccab7d))
* **api:** preserve offline presence in snapshots ([#1172](https://github.com/chattocorp/chatto/issues/1172)) ([7fce244](https://github.com/chattocorp/chatto/commit/7fce244d8f7deecd821966923ce2992c5a656f2c))
* **api:** tighten ConnectRPC caller auth ([#1126](https://github.com/chattocorp/chatto/issues/1126)) ([bb8c10d](https://github.com/chattocorp/chatto/commit/bb8c10df48a2c7e8a9a94164ee66d24d0517ac31))
* **assets:** prevent protected attachment caching ([#1261](https://github.com/chattocorp/chatto/issues/1261)) ([e3c6eed](https://github.com/chattocorp/chatto/commit/e3c6eedab25aa279ca1cae8e3ea2497fe391053d))
* **assets:** serve protected assets through stable gateway ([#1264](https://github.com/chattocorp/chatto/issues/1264)) ([744e93e](https://github.com/chattocorp/chatto/commit/744e93ed552e3920df9a76e0c3b6c9a90ebf6dcd))
* **attachments:** crop extreme image thumbnails ([#1181](https://github.com/chattocorp/chatto/issues/1181)) ([d5dd244](https://github.com/chattocorp/chatto/commit/d5dd244e42ea884cf4739523cda3479a17c1e4f8))
* **auth:** add structured unauthenticated GraphQL errors ([#1048](https://github.com/chattocorp/chatto/issues/1048)) ([510c07d](https://github.com/chattocorp/chatto/commit/510c07dd38ad3ccc9e87f515878c96594c72c9dd))
* **auth:** reject empty-user runtime credentials ([#1201](https://github.com/chattocorp/chatto/issues/1201)) ([43b569c](https://github.com/chattocorp/chatto/commit/43b569c348c89cdf6df1f49a6433b385625a2589))
* **calls:** preserve call on tab takeover ([#1284](https://github.com/chattocorp/chatto/issues/1284)) ([451929d](https://github.com/chattocorp/chatto/commit/451929d2bf2dbbcffbc879a5e98ceac2ab1153b1))
* **ci:** gate release-please on green ci ([#1135](https://github.com/chattocorp/chatto/issues/1135)) ([4decb0f](https://github.com/chattocorp/chatto/commit/4decb0f1362e876e461ce9436a6ce0f8cb340eab))
* **conductor:** use workspace port for Storybook ([#1290](https://github.com/chattocorp/chatto/issues/1290)) ([c5ba4dc](https://github.com/chattocorp/chatto/commit/c5ba4dc775c611e27c916cb46179a7d8264ac8f9))
* **connectapi:** harden message post migration ([#1097](https://github.com/chattocorp/chatto/issues/1097)) ([b15fb14](https://github.com/chattocorp/chatto/commit/b15fb14c2ee708915ab79255f6a86aab3c4cc764))
* **connectapi:** harden timeline and thread read handling ([#1117](https://github.com/chattocorp/chatto/issues/1117)) ([ba027fe](https://github.com/chattocorp/chatto/commit/ba027fe3b7727620307bc4936633effe8abd255d))
* **connectrpc:** cap request message size ([#1102](https://github.com/chattocorp/chatto/issues/1102)) ([a773531](https://github.com/chattocorp/chatto/commit/a773531e687de72645ee78b1aa09f07f9d61ef61))
* **connectrpc:** reject missing read anchors ([#1109](https://github.com/chattocorp/chatto/issues/1109)) ([f2f68b9](https://github.com/chattocorp/chatto/commit/f2f68b96fca00c177975600f1e9f38f2787a3c4b))
* **core:** complete service inventory metrics ([#1130](https://github.com/chattocorp/chatto/issues/1130)) ([9bc89f3](https://github.com/chattocorp/chatto/commit/9bc89f3e116df73330be22484b13a999419b12ed))
* **core:** prevent read marker regressions ([#1107](https://github.com/chattocorp/chatto/issues/1107)) ([cb81d58](https://github.com/chattocorp/chatto/commit/cb81d583f9c789319790109624af5ad8d112d680))
* **dockercompose:** enable LiveKit TURN relay ([#1190](https://github.com/chattocorp/chatto/issues/1190)) ([51eb5e7](https://github.com/chattocorp/chatto/commit/51eb5e799f4ebabb395c9f5073219d4015b2ac10))
* **docs:** keep release note cards in grid lanes ([#1204](https://github.com/chattocorp/chatto/issues/1204)) ([a6c79df](https://github.com/chattocorp/chatto/commit/a6c79df79793e9e3927a7d738b4f54ddbc1940f9))
* **frontend:** address svelte guidance review ([#1154](https://github.com/chattocorp/chatto/issues/1154)) ([d8c4010](https://github.com/chattocorp/chatto/commit/d8c4010b1b02ec4b65a15408b07f3800180a2a5e))
* **frontend:** align call control button colors ([#1085](https://github.com/chattocorp/chatto/issues/1085)) ([4b7f37e](https://github.com/chattocorp/chatto/commit/4b7f37e87d1bcfe8b388f59aa1ae70b7e3aff5ea))
* **frontend:** align muted call participant icon ([#1050](https://github.com/chattocorp/chatto/issues/1050)) ([68cea04](https://github.com/chattocorp/chatto/commit/68cea040f6129134b50cf1c745274e3f669b3746))
* **frontend:** clarify echo reply actions ([#1253](https://github.com/chattocorp/chatto/issues/1253)) ([5a2b264](https://github.com/chattocorp/chatto/commit/5a2b2645bd046c3e925bbb2c24c47eecbe534589))
* **frontend:** clarify iOS PWA push setup ([#1192](https://github.com/chattocorp/chatto/issues/1192)) ([2416a41](https://github.com/chattocorp/chatto/commit/2416a41f1cbf3cf31038b087c8cc207de8967c5e))
* **frontend:** clarify remote push notification support ([#1105](https://github.com/chattocorp/chatto/issues/1105)) ([bfdbdea](https://github.com/chattocorp/chatto/commit/bfdbdea4050d529ba060f5931009d74026a8631f))
* **frontend:** clear call-wide mode on notification navigation ([#1291](https://github.com/chattocorp/chatto/issues/1291)) ([db09a62](https://github.com/chattocorp/chatto/commit/db09a62949ffdbe56a7b1436a3a10df901b889bf))
* **frontend:** constrain current user card height ([#1239](https://github.com/chattocorp/chatto/issues/1239)) ([1b536b9](https://github.com/chattocorp/chatto/commit/1b536b96a7d0c7abb6baa152d82d348e0f6b0218))
* **frontend:** defer camera permission until enabled ([#1243](https://github.com/chattocorp/chatto/issues/1243)) ([2145a95](https://github.com/chattocorp/chatto/commit/2145a9535ada73b05a0938b5b6249c264eed99d1))
* **frontend:** defer unread separator until return to the room ([#1079](https://github.com/chattocorp/chatto/issues/1079)) ([9535694](https://github.com/chattocorp/chatto/commit/95356945a66376560017888ef0291295f6d13f1e))
* **frontend:** handle API auth failures gracefully ([#1269](https://github.com/chattocorp/chatto/issues/1269)) ([e82c554](https://github.com/chattocorp/chatto/commit/e82c5543328b6999ec65b4ede625cd28b17b89b9))
* **frontend:** harden asset proxy token handling ([#1054](https://github.com/chattocorp/chatto/issues/1054)) ([8797c65](https://github.com/chattocorp/chatto/commit/8797c65aa35b304ac5e77216f783f404865d2928))
* **frontend:** ignore stale DM member loads when switching rooms ([#1065](https://github.com/chattocorp/chatto/issues/1065)) ([b4264b7](https://github.com/chattocorp/chatto/commit/b4264b77c12b4492b0391597072e20a1809b0316))
* **frontend:** improve call presence indicators ([#1257](https://github.com/chattocorp/chatto/issues/1257)) ([696a92e](https://github.com/chattocorp/chatto/commit/696a92e008919c2358c188a23963bc9d489fc166))
* **frontend:** improve extreme image thumbnails ([#1227](https://github.com/chattocorp/chatto/issues/1227)) ([d5c596d](https://github.com/chattocorp/chatto/commit/d5c596d56bb306e4503c36d1883900b284d7b5c7))
* **frontend:** improve LiveKit media error handling ([#1281](https://github.com/chattocorp/chatto/issues/1281)) ([94a86c0](https://github.com/chattocorp/chatto/commit/94a86c0e9ed789f7e05175186b7ecfdd999af1bf))
* **frontend:** improve unread channel contrast ([#1089](https://github.com/chattocorp/chatto/issues/1089)) ([74247b4](https://github.com/chattocorp/chatto/commit/74247b42833d07c33a2950dc357cf5c4b06a3f66))
* **frontend:** localize date formatting ([#1242](https://github.com/chattocorp/chatto/issues/1242)) ([cfc96ec](https://github.com/chattocorp/chatto/commit/cfc96ec847220f580249031d25f5db80dbd89ecf))
* **frontend:** make attachment remove control subtle ([#1265](https://github.com/chattocorp/chatto/issues/1265)) ([6537c27](https://github.com/chattocorp/chatto/commit/6537c2768136e633fdea9d36226ab9fb350b8875))
* **frontend:** make scrollbars follow selected theme ([#1152](https://github.com/chattocorp/chatto/issues/1152)) ([9c5fa16](https://github.com/chattocorp/chatto/commit/9c5fa16da9555d38c0331e5876d4b35b025d4371))
* **frontend:** polish error and missing media states ([#1267](https://github.com/chattocorp/chatto/issues/1267)) ([b9dabba](https://github.com/chattocorp/chatto/commit/b9dabba0f018656fb46418868ed65e3774bea627))
* **frontend:** preserve touch composer line breaks ([#1194](https://github.com/chattocorp/chatto/issues/1194)) ([8c62c70](https://github.com/chattocorp/chatto/commit/8c62c700f1a4a07369cb17ba0dd2ea9141bcdf8d))
* **frontend:** quiet console warning noise ([#1280](https://github.com/chattocorp/chatto/issues/1280)) ([4df7b85](https://github.com/chattocorp/chatto/commit/4df7b8575f1bf489d6f0518e31367dfb8729af7a))
* **frontend:** reconcile notification badge dismissals ([#1058](https://github.com/chattocorp/chatto/issues/1058)) ([13c7a6e](https://github.com/chattocorp/chatto/commit/13c7a6ef51a34f6a99964fcbe167f30fd8e7d304))
* **frontend:** reconcile PWA notification badges ([#1229](https://github.com/chattocorp/chatto/issues/1229)) ([e44645e](https://github.com/chattocorp/chatto/commit/e44645e271cf099eec2e19f9030b10891f76f937))
* **frontend:** refresh messages after local deletions ([#1148](https://github.com/chattocorp/chatto/issues/1148)) ([cefc22a](https://github.com/chattocorp/chatto/commit/cefc22a77efee0f333b848a054c0a56078b0a0d6))
* **frontend:** remove redundant universal room badge ([#1052](https://github.com/chattocorp/chatto/issues/1052)) ([5f6131e](https://github.com/chattocorp/chatto/commit/5f6131ee3fe98e5713a2eb64e2da22f5d5287e68))
* **frontend:** reset inline code state when composer clears ([#1251](https://github.com/chattocorp/chatto/issues/1251)) ([0dddeaa](https://github.com/chattocorp/chatto/commit/0dddeaa24e62d028797a93f3cd808e94a1141485))
* **frontend:** restore circular avatars with stable presence dots ([#1252](https://github.com/chattocorp/chatto/issues/1252)) ([14b15b9](https://github.com/chattocorp/chatto/commit/14b15b93382c9b9719b068e889691b5f44f6cf2f))
* **frontend:** restore default text smoothing ([#1268](https://github.com/chattocorp/chatto/issues/1268)) ([b3a6dc3](https://github.com/chattocorp/chatto/commit/b3a6dc3c2796181995338649e4a2a7502e56761b))
* **frontend:** restrict same-tab message links ([#1068](https://github.com/chattocorp/chatto/issues/1068)) ([d43d23f](https://github.com/chattocorp/chatto/commit/d43d23f70da28a324743673f585085c70f5d89ac))
* **frontend:** restyle reply attribution preview ([#1140](https://github.com/chattocorp/chatto/issues/1140)) ([909c1f4](https://github.com/chattocorp/chatto/commit/909c1f4a2d67ba2979be765b9eaecff611e96e90))
* **frontend:** share unread marker lifecycle with threads ([#1310](https://github.com/chattocorp/chatto/issues/1310)) ([07c3601](https://github.com/chattocorp/chatto/commit/07c36016132af7dd34441f6e032240f6e03bf721))
* **frontend:** show loading state for call media toggles ([#1237](https://github.com/chattocorp/chatto/issues/1237)) ([9063832](https://github.com/chattocorp/chatto/commit/9063832ae074a47852f340b38fa15d755c8399a6))
* **frontend:** stabilize new messages separator ([#1308](https://github.com/chattocorp/chatto/issues/1308)) ([c35ed86](https://github.com/chattocorp/chatto/commit/c35ed8641c9910bf86c71e38563a42868e3cc2a4))
* **frontend:** stabilize tab resume catch-up ([#1288](https://github.com/chattocorp/chatto/issues/1288)) ([b70916d](https://github.com/chattocorp/chatto/commit/b70916d877c231dba9ab67fbfc2983df4d774aa2))
* **frontend:** style room member search clear button ([#1226](https://github.com/chattocorp/chatto/issues/1226)) ([e43f615](https://github.com/chattocorp/chatto/commit/e43f615e951b12200f1994e844d3b82de4ecdeca))
* **frontend:** submit simple message edits with enter ([#1129](https://github.com/chattocorp/chatto/issues/1129)) ([f5651b4](https://github.com/chattocorp/chatto/commit/f5651b4413b70aaa954d3bdb7c553df21e7c42ca))
* **frontend:** sync presence badge across tabs ([#1301](https://github.com/chattocorp/chatto/issues/1301)) ([5fbfb22](https://github.com/chattocorp/chatto/commit/5fbfb22d715f6a315f61a0c9f3a063879842468b))
* **frontend:** sync room thread follow bell state ([#1121](https://github.com/chattocorp/chatto/issues/1121)) ([4048f23](https://github.com/chattocorp/chatto/commit/4048f23256f87e417509fb887d2919c59bad5a38))
* **frontend:** use direct ticketed asset URLs ([#1312](https://github.com/chattocorp/chatto/issues/1312)) ([b41eb1d](https://github.com/chattocorp/chatto/commit/b41eb1d8d3d062f794c680a892bed15a5d451ca3))
* **frontend:** use full-width image galleries ([#1247](https://github.com/chattocorp/chatto/issues/1247)) ([f5fe88a](https://github.com/chattocorp/chatto/commit/f5fe88aff3fdfc9cc676dd8735dfd850fc3a7cb3))
* **frontend:** use semantic presence colors ([#1259](https://github.com/chattocorp/chatto/issues/1259)) ([ccf64db](https://github.com/chattocorp/chatto/commit/ccf64db80552887782a357b3bb23acdba7f12b0c))
* **frontend:** wire UI strings to i18n ([#1225](https://github.com/chattocorp/chatto/issues/1225)) ([7eafcd3](https://github.com/chattocorp/chatto/commit/7eafcd34507e6a86e4983ac2ab29c25ee0e6cb95))
* **media:** preserve video aspect ratios ([#1254](https://github.com/chattocorp/chatto/issues/1254)) ([8a85f0a](https://github.com/chattocorp/chatto/commit/8a85f0a434e688fe2a7b25a096c010ee74ebd274))
* **messages:** validate reply targets before posting ([#1176](https://github.com/chattocorp/chatto/issues/1176)) ([2919a1a](https://github.com/chattocorp/chatto/commit/2919a1a4fcb0cf5b13a6e22764329bee0f9f1d1d))
* **notifications:** clear read notifications server-side ([#1297](https://github.com/chattocorp/chatto/issues/1297)) ([c6f3c30](https://github.com/chattocorp/chatto/commit/c6f3c30d1729f48bebf47047963262acd32a1d4e))
* **notifications:** preserve unread badge state across dismissals ([#1069](https://github.com/chattocorp/chatto/issues/1069)) ([03444e3](https://github.com/chattocorp/chatto/commit/03444e39cf171bb87277d6db20fd20d422378a3d))
* **pwa:** reduce service worker reload churn ([#1187](https://github.com/chattocorp/chatto/issues/1187)) ([5489e47](https://github.com/chattocorp/chatto/commit/5489e4742cf577f50295dc8f29d30ed64841245b))
* **reactions:** canonicalize echo reaction targets ([#1272](https://github.com/chattocorp/chatto/issues/1272)) ([2b87044](https://github.com/chattocorp/chatto/commit/2b8704479e08eb66ebefc86243cd3f8aa98d338b))
* **release:** publish release before updating tap ([#1298](https://github.com/chattocorp/chatto/issues/1298)) ([c5c8aa6](https://github.com/chattocorp/chatto/commit/c5c8aa64b255b423a2b01c074f1e7155a2a7f3ef))
* **voice:** scope LiveKit observations to active calls ([#1049](https://github.com/chattocorp/chatto/issues/1049)) ([dcd95c8](https://github.com/chattocorp/chatto/commit/dcd95c8cdd9f964e36eeea73592d2827dcb83c9e))


### Performance Improvements

* **build:** improve frontend and CLI cache reuse ([#1106](https://github.com/chattocorp/chatto/issues/1106)) ([f22da3a](https://github.com/chattocorp/chatto/commit/f22da3adcd5a8affe8b15715cd02569baddad2e7))
* **core:** cache unwrapped DEKs per request ([#1193](https://github.com/chattocorp/chatto/issues/1193)) ([0623831](https://github.com/chattocorp/chatto/commit/0623831519d7e4839caa77f18fe0a7702e604305))
* **core:** slim timeline projection memory ([#1287](https://github.com/chattocorp/chatto/issues/1287)) ([cd026ff](https://github.com/chattocorp/chatto/commit/cd026ff14dab59b45d9bf26b78bcdca08b81edc8))
* **frontend:** load room members in larger batches ([#1206](https://github.com/chattocorp/chatto/issues/1206)) ([f465a09](https://github.com/chattocorp/chatto/commit/f465a095e88819c6f210f36b1bc334e3c4e06c5a))
* **frontend:** split chat code from app chrome ([#1103](https://github.com/chattocorp/chatto/issues/1103)) ([4a4a4de](https://github.com/chattocorp/chatto/commit/4a4a4de0747e73d37183bc3fde89f6d0f45c8890))


### Code Refactoring

* **api:** consolidate ConnectRPC surface ([#1306](https://github.com/chattocorp/chatto/issues/1306)) ([900233d](https://github.com/chattocorp/chatto/commit/900233da483c64de8aa6f7fd2d6d7a6d6f2cc16b))
* **api:** consolidate public ConnectRPC API ([#1295](https://github.com/chattocorp/chatto/issues/1295)) ([a0ab823](https://github.com/chattocorp/chatto/commit/a0ab82321db80f44569fd55019726b8e4c458ddb))

## [0.4.0-beta.14](https://github.com/chattocorp/chatto/compare/v0.4.0-beta.13...v0.4.0-beta.14) (2026-07-05)


### ⚠ BREAKING CHANGES

* **api:** consolidate ConnectRPC surface ([#1306](https://github.com/chattocorp/chatto/issues/1306))
* **api:** clean up server assets calls and includes ([#1303](https://github.com/chattocorp/chatto/issues/1303))
* **api:** consolidate shared api shapes ([#1302](https://github.com/chattocorp/chatto/issues/1302))
* **api:** consolidate shared public API types ([#1299](https://github.com/chattocorp/chatto/issues/1299))

### Features

* **api:** clean up server assets calls and includes ([#1303](https://github.com/chattocorp/chatto/issues/1303)) ([e960def](https://github.com/chattocorp/chatto/commit/e960defc9c3a1cc77ae1958a8c98d9cc54919c25))
* **api:** consolidate shared api shapes ([#1302](https://github.com/chattocorp/chatto/issues/1302)) ([4429009](https://github.com/chattocorp/chatto/commit/4429009ba3dd1b4ce0800b928c55fb8eaa308376))
* **api:** consolidate shared public API types ([#1299](https://github.com/chattocorp/chatto/issues/1299)) ([1ec2015](https://github.com/chattocorp/chatto/commit/1ec201551881142d8d5498902d0ff192e7b8bf7e))


### Bug Fixes

* **frontend:** sync presence badge across tabs ([#1301](https://github.com/chattocorp/chatto/issues/1301)) ([5fbfb22](https://github.com/chattocorp/chatto/commit/5fbfb22d715f6a315f61a0c9f3a063879842468b))
* **release:** publish release before updating tap ([#1298](https://github.com/chattocorp/chatto/issues/1298)) ([c5c8aa6](https://github.com/chattocorp/chatto/commit/c5c8aa64b255b423a2b01c074f1e7155a2a7f3ef))


### Code Refactoring

* **api:** consolidate ConnectRPC surface ([#1306](https://github.com/chattocorp/chatto/issues/1306)) ([900233d](https://github.com/chattocorp/chatto/commit/900233da483c64de8aa6f7fd2d6d7a6d6f2cc16b))

## [0.4.0-beta.13](https://github.com/chattocorp/chatto/compare/v0.4.0-beta.12...v0.4.0-beta.13) (2026-07-04)


### ⚠ BREAKING CHANGES

* **api:** consolidate public ConnectRPC API ([#1295](https://github.com/chattocorp/chatto/issues/1295))

### Features

* **api:** consolidate membership services ([#1293](https://github.com/chattocorp/chatto/issues/1293)) ([7ed268c](https://github.com/chattocorp/chatto/commit/7ed268c71443c75201a1d26036f318f8df6f6e05))


### Bug Fixes

* **calls:** preserve call on tab takeover ([#1284](https://github.com/chattocorp/chatto/issues/1284)) ([451929d](https://github.com/chattocorp/chatto/commit/451929d2bf2dbbcffbc879a5e98ceac2ab1153b1))
* **conductor:** use workspace port for Storybook ([#1290](https://github.com/chattocorp/chatto/issues/1290)) ([c5ba4dc](https://github.com/chattocorp/chatto/commit/c5ba4dc775c611e27c916cb46179a7d8264ac8f9))
* **frontend:** clear call-wide mode on notification navigation ([#1291](https://github.com/chattocorp/chatto/issues/1291)) ([db09a62](https://github.com/chattocorp/chatto/commit/db09a62949ffdbe56a7b1436a3a10df901b889bf))
* **frontend:** improve LiveKit media error handling ([#1281](https://github.com/chattocorp/chatto/issues/1281)) ([94a86c0](https://github.com/chattocorp/chatto/commit/94a86c0e9ed789f7e05175186b7ecfdd999af1bf))
* **frontend:** quiet console warning noise ([#1280](https://github.com/chattocorp/chatto/issues/1280)) ([4df7b85](https://github.com/chattocorp/chatto/commit/4df7b8575f1bf489d6f0518e31367dfb8729af7a))
* **frontend:** stabilize tab resume catch-up ([#1288](https://github.com/chattocorp/chatto/issues/1288)) ([b70916d](https://github.com/chattocorp/chatto/commit/b70916d877c231dba9ab67fbfc2983df4d774aa2))
* **notifications:** clear read notifications server-side ([#1297](https://github.com/chattocorp/chatto/issues/1297)) ([c6f3c30](https://github.com/chattocorp/chatto/commit/c6f3c30d1729f48bebf47047963262acd32a1d4e))


### Performance Improvements

* **core:** slim timeline projection memory ([#1287](https://github.com/chattocorp/chatto/issues/1287)) ([cd026ff](https://github.com/chattocorp/chatto/commit/cd026ff14dab59b45d9bf26b78bcdca08b81edc8))


### Code Refactoring

* **api:** consolidate public ConnectRPC API ([#1295](https://github.com/chattocorp/chatto/issues/1295)) ([a0ab823](https://github.com/chattocorp/chatto/commit/a0ab82321db80f44569fd55019726b8e4c458ddb))

## [0.4.0-beta.12](https://github.com/chattocorp/chatto/compare/v0.4.0-beta.11...v0.4.0-beta.12) (2026-07-03)


### Features

* **frontend:** refresh toast styling ([#1260](https://github.com/chattocorp/chatto/issues/1260)) ([1b728e5](https://github.com/chattocorp/chatto/commit/1b728e511e6b6310d10d59bf7c6085d4c70710d0))


### Bug Fixes

* **assets:** prevent protected attachment caching ([#1261](https://github.com/chattocorp/chatto/issues/1261)) ([e3c6eed](https://github.com/chattocorp/chatto/commit/e3c6eedab25aa279ca1cae8e3ea2497fe391053d))
* **assets:** serve protected assets through stable gateway ([#1264](https://github.com/chattocorp/chatto/issues/1264)) ([744e93e](https://github.com/chattocorp/chatto/commit/744e93ed552e3920df9a76e0c3b6c9a90ebf6dcd))
* **frontend:** handle API auth failures gracefully ([#1269](https://github.com/chattocorp/chatto/issues/1269)) ([e82c554](https://github.com/chattocorp/chatto/commit/e82c5543328b6999ec65b4ede625cd28b17b89b9))
* **frontend:** make attachment remove control subtle ([#1265](https://github.com/chattocorp/chatto/issues/1265)) ([6537c27](https://github.com/chattocorp/chatto/commit/6537c2768136e633fdea9d36226ab9fb350b8875))
* **frontend:** polish error and missing media states ([#1267](https://github.com/chattocorp/chatto/issues/1267)) ([b9dabba](https://github.com/chattocorp/chatto/commit/b9dabba0f018656fb46418868ed65e3774bea627))
* **frontend:** restore default text smoothing ([#1268](https://github.com/chattocorp/chatto/issues/1268)) ([b3a6dc3](https://github.com/chattocorp/chatto/commit/b3a6dc3c2796181995338649e4a2a7502e56761b))
* **frontend:** use semantic presence colors ([#1259](https://github.com/chattocorp/chatto/issues/1259)) ([ccf64db](https://github.com/chattocorp/chatto/commit/ccf64db80552887782a357b3bb23acdba7f12b0c))
* **reactions:** canonicalize echo reaction targets ([#1272](https://github.com/chattocorp/chatto/issues/1272)) ([2b87044](https://github.com/chattocorp/chatto/commit/2b8704479e08eb66ebefc86243cd3f8aa98d338b))

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
