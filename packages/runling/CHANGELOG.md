# Changelog

## [0.7.0](https://github.com/chattocorp/runling/compare/v0.6.0...v0.7.0) (2026-09-16)


### ⚠ BREAKING CHANGES

* compose workflow input handlers and add a Chatto demo ([#30](https://github.com/chattocorp/runling/issues/30))
* restore explicit workflow context for tasks and agents ([#28](https://github.com/chattocorp/runling/issues/28))
* remove task context and require explicit directories ([#26](https://github.com/chattocorp/runling/issues/26))
* add run and serve commands and align task naming ([#25](https://github.com/chattocorp/runling/issues/25))
* rename workflow wrapper to task ([#15](https://github.com/chattocorp/runling/issues/15))

### Features

* add Chatto planning demo with routed input and live steering ([#34](https://github.com/chattocorp/runling/issues/34)) ([8beab0e](https://github.com/chattocorp/runling/commit/8beab0ed87de9ec40196d9181afdb59518f37eca))
* add persistent Chatto agent conversations ([#38](https://github.com/chattocorp/runling/issues/38)) ([546cb3d](https://github.com/chattocorp/runling/commit/546cb3d239cf751532450dced8066acc5ecf6e16))
* add run and serve commands and align task naming ([#25](https://github.com/chattocorp/runling/issues/25)) ([6df3f49](https://github.com/chattocorp/runling/commit/6df3f496d1b6838362b71895e7e5228a0140e654))
* add task channels, agent tools, and function webhook routers ([#37](https://github.com/chattocorp/runling/issues/37)) ([7876e0f](https://github.com/chattocorp/runling/commit/7876e0f99d70b7c3d61d6700d4425f3faa31945f))
* add workflow and input timeouts ([#31](https://github.com/chattocorp/runling/issues/31)) ([67f558d](https://github.com/chattocorp/runling/commit/67f558ddea033405e69194ec2cbccd6e4e4651e2))
* add workflow cancellation with a compact header action ([#35](https://github.com/chattocorp/runling/issues/35)) ([0128d18](https://github.com/chattocorp/runling/commit/0128d18c76ee2edfd72f1ea8a47b5fcb90d804c8))
* compose workflow input handlers and add a Chatto demo ([#30](https://github.com/chattocorp/runling/issues/30)) ([63b2535](https://github.com/chattocorp/runling/commit/63b253513ef1ddd6a7ab43a84760074342410ecd))
* expose git helpers through runling/git ([#24](https://github.com/chattocorp/runling/issues/24)) ([a9b973f](https://github.com/chattocorp/runling/commit/a9b973fadaef7f8d2a6cc55860317f347439b6e6))
* let tasks abort workflows through their context ([#29](https://github.com/chattocorp/runling/issues/29)) ([8a23ec4](https://github.com/chattocorp/runling/commit/8a23ec4ffa4d69a74746b47651220c385b56fe4f))
* remove task context and require explicit directories ([#26](https://github.com/chattocorp/runling/issues/26)) ([b788377](https://github.com/chattocorp/runling/commit/b7883779e83ce3006e6e00c71501c8e00341e71e))
* rename workflow wrapper to task ([#15](https://github.com/chattocorp/runling/issues/15)) ([3e638b7](https://github.com/chattocorp/runling/commit/3e638b7dbe7c9ef4be23eb11bac34324860966e0))
* restore explicit workflow context for tasks and agents ([#28](https://github.com/chattocorp/runling/issues/28)) ([eeefa13](https://github.com/chattocorp/runling/commit/eeefa13a57945011058fbf518559e26e073072ae))
* support Standard Schema task validation ([#27](https://github.com/chattocorp/runling/issues/27)) ([761c800](https://github.com/chattocorp/runling/commit/761c80057f340793ee6a4c1ecab0ad18224afcb9))


### Bug Fixes

* clarify run inspector empty state ([#33](https://github.com/chattocorp/runling/issues/33)) ([18b5f39](https://github.com/chattocorp/runling/commit/18b5f3933652b7ee69032955a9d39e3d2182c3d2))
* dock timeline overview below the chart ([#36](https://github.com/chattocorp/runling/issues/36)) ([149f2de](https://github.com/chattocorp/runling/commit/149f2dee4b0f07b429559b6ba3f89cec7d823010))
* include committed tree changes in Git snapshots ([#20](https://github.com/chattocorp/runling/issues/20)) ([3df09b8](https://github.com/chattocorp/runling/commit/3df09b8cd381bd1fc460b1c84b7dd410859b5e30))
* move timeline minimap to upper right ([#32](https://github.com/chattocorp/runling/issues/32)) ([72676f1](https://github.com/chattocorp/runling/commit/72676f17e9516aa3a18475285d63e1ed04d58e7e))
* pin web fetch connections to validated DNS addresses ([#22](https://github.com/chattocorp/runling/issues/22)) ([6bb7591](https://github.com/chattocorp/runling/commit/6bb7591e18ec143a2bf16501d76160c6c22a9747))
* preserve forked conversation history during compaction ([#19](https://github.com/chattocorp/runling/issues/19)) ([4fd68d0](https://github.com/chattocorp/runling/commit/4fd68d0fb27f4bef50b30d2f9e1202f119494242))


### Performance Improvements

* load completed run details on demand ([#21](https://github.com/chattocorp/runling/issues/21)) ([e33bc2a](https://github.com/chattocorp/runling/commit/e33bc2ae424b80c31456ac9433df3d03275ebbab))

## [0.6.0](https://github.com/chattocorp/runling/compare/v0.5.0...v0.6.0) (2026-09-07)


### Features

* add persistent column resizing ([c7346ad](https://github.com/chattocorp/runling/commit/c7346ad785b69d861b01cf0836df6dc00f5e2f5c))
* dim and disable UI while disconnected ([206bba8](https://github.com/chattocorp/runling/commit/206bba8af5c8dd2bdbb64329c93f23d159b50820))
* show live activity in recent runs ([ba5cfa5](https://github.com/chattocorp/runling/commit/ba5cfa5f0cfb1dec8b5760c662b9a30b3d70d8b9))
* unify input output and logs panels ([69f1fb3](https://github.com/chattocorp/runling/commit/69f1fb349130172a9d9ef5451a3204a3f715f909))
* use braille indicators for recent runs ([2983c65](https://github.com/chattocorp/runling/commit/2983c65865aa03b1e690e4347647042168b31026))


### Bug Fixes

* log when the web server starts shutting down ([31298b7](https://github.com/chattocorp/runling/commit/31298b746d638754175394538e1f492ebf020ee5))

## [0.5.0](https://github.com/chattocorp/runling/compare/v0.4.1...v0.5.0) (2026-09-06)


### ⚠ BREAKING CHANGES

* make run usage accounting controls internal
* slim runtime API and scope usage totals to runs

### Features

* start web server without a config file ([1c8eb61](https://github.com/chattocorp/runling/commit/1c8eb61d931eb23438ebdb35ab9ceb73a3551e5f))
* **web:** improve output tab and remove duplicate timeline output ([53d6f55](https://github.com/chattocorp/runling/commit/53d6f55373b97fca32b7f97ee131c05d55d08e5c))
* **web:** inspect activities in a modal and add form submit shortcut ([ab5bb85](https://github.com/chattocorp/runling/commit/ab5bb852a4f0f2720659b930cd1d153ec86a7786))
* **web:** migrate UI to Tailwind and DaisyUI ([9d29f15](https://github.com/chattocorp/runling/commit/9d29f159205951053fc06327bb811932635a4f01))
* **web:** show Braille spinner on active timeline tasks ([74cb237](https://github.com/chattocorp/runling/commit/74cb23760eef29c0e8ee30637079c8d4b195f2bc))
* **web:** streamline sidebar layout and run summaries ([4fad0eb](https://github.com/chattocorp/runling/commit/4fad0eb2900570a71774e5a40ee3c1971c686d09))


### Bug Fixes

* prepare SvelteKit files for tests and update CLI validation test ([a8c92ac](https://github.com/chattocorp/runling/commit/a8c92acca4de418ebf00514d3e0c0661c5aacae7))
* **web:** bound timeline zoom to actual activity endpoints ([b27483b](https://github.com/chattocorp/runling/commit/b27483bd83e44964753cf9d9f3a111000b64dcd4))
* **web:** give timeline labels a two-line layout ([48ac84e](https://github.com/chattocorp/runling/commit/48ac84e531b9989434442ca9aa5def7626ce7f23))


### Code Refactoring

* make run usage accounting controls internal ([c9c9945](https://github.com/chattocorp/runling/commit/c9c99450c01e6dd0a089b1f28def2a476903c75b))
* slim runtime API and scope usage totals to runs ([5bc51f5](https://github.com/chattocorp/runling/commit/5bc51f500556d8a2bc2bbfb7ed57c08a276d405d))

## [0.4.1](https://github.com/chattocorp/runling/compare/v0.4.0...v0.4.1) (2026-09-05)


### Bug Fixes

* **ci:** simplify tests and publish an explicit local tarball ([d12780a](https://github.com/chattocorp/runling/commit/d12780a8d80029e4aeddf40fc701e19e1bc381f5))

## [0.4.0](https://github.com/chattocorp/runling/compare/v0.3.0...v0.4.0) (2026-09-05)


### Features

* **web:** add persistent sidebar toggle ([39e99ec](https://github.com/chattocorp/runling/commit/39e99ecf5f39fbe570f454eb38598bd69e4cbf06))
* **web:** give timeline bars a subtle raised finish ([c7e943e](https://github.com/chattocorp/runling/commit/c7e943e07a05ff21ab77d04a917cc4ea4ae74b53))
* **web:** toggle sidebars with Command or Control B ([0988c8c](https://github.com/chattocorp/runling/commit/0988c8c603f066a715be3ea6725b9e68ad05cc47))


### Bug Fixes

* stop consumer process tree and retry smoke test cleanup ([ff25300](https://github.com/chattocorp/runling/commit/ff25300a3b517f9c1d6979818db1e10ba1095081))
* **web:** place sidebar toggle beside the logo ([996b47e](https://github.com/chattocorp/runling/commit/996b47ebdb6e0b276c7fba81b6b2f11f1c3d4b9d))
