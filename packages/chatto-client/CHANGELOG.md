# Changelog

## 0.1.1 (2026-10-23)

### Maintenance

* Publish through GitHub Actions with npm trusted publishing after the manual 0.1.0 release.

## 0.1.0 (2026-10-23)


### ⚠ BREAKING CHANGES

* **permissions:** load complete permission matrices ([#2966](https://github.com/chattocorp/chatto/issues/2966))
* **uploads:** use one size limit for all attachments ([#2958](https://github.com/chattocorp/chatto/issues/2958))
* **permissions:** add dedicated message.pin permission ([#2938](https://github.com/chattocorp/chatto/issues/2938))
* **messages:** separate posting from read state ([#2868](https://github.com/chattocorp/chatto/issues/2868))

### Features

* **chat:** add independent channel and thread slow mode ([#3036](https://github.com/chattocorp/chatto/issues/3036)) ([285587a](https://github.com/chattocorp/chatto/commit/285587a7cb542693eb2b692845ad2ffa484b2055))
* **chattobot:** implement changes through green CI with supervisor-led updates ([#2746](https://github.com/chattocorp/chatto/issues/2746)) ([936b7c8](https://github.com/chattocorp/chatto/commit/936b7c802d82b2bc123cd584290e764d936c814d))
* **chattobot:** view image and text attachments ([#2776](https://github.com/chattocorp/chatto/issues/2776)) ([8ff9d87](https://github.com/chattocorp/chatto/commit/8ff9d8720668cc0992de5e138ea7dde43c4f8590))
* **client:** allow best-effort access to older remote servers ([#3008](https://github.com/chattocorp/chatto/issues/3008)) ([af103a3](https://github.com/chattocorp/chatto/commit/af103a36c9c592fbeef20d5f1b16334b670f3ac7))
* **client:** extract shared Chatto integration helpers ([#2518](https://github.com/chattocorp/chatto/issues/2518)) ([4cd7063](https://github.com/chattocorp/chatto/commit/4cd70636c1405c4f65ac2ff72a51641dee23e9f3))
* **client:** move the frontend client state layer into @chatto/client ([#2749](https://github.com/chattocorp/chatto/issues/2749)) ([39f6921](https://github.com/chattocorp/chatto/commit/39f69216cc92af5c4779b2c03250048e01484ab5))
* **client:** publish Apache-2.0 client packages to npm ([#3135](https://github.com/chattocorp/chatto/issues/3135)) ([181a521](https://github.com/chattocorp/chatto/commit/181a521e60238be52d2abf1493a1c0ef929724b3))
* extract chatto-client-placement agentic skill ([#2808](https://github.com/chattocorp/chatto/issues/2808)) ([cbe9353](https://github.com/chattocorp/chatto/commit/cbe9353c9f5b2845e27080bcd1fd2aa2095554d1))
* **frontend:** add server information and feature compatibility ([#3027](https://github.com/chattocorp/chatto/issues/3027)) ([d70693d](https://github.com/chattocorp/chatto/commit/d70693d96dbf6e1450c0261cf2bdac42b784c239))
* **frontend:** expand pinned messages into room information ([#3131](https://github.com/chattocorp/chatto/issues/3131)) ([7a31dd6](https://github.com/chattocorp/chatto/commit/7a31dd6b34deee543c3e3717eeaa17a8ad8a0b3a))
* **frontend:** keep recent and unread DMs in the sidebar ([#3087](https://github.com/chattocorp/chatto/issues/3087)) ([1d8b197](https://github.com/chattocorp/chatto/commit/1d8b1977fdb8de95f57af42eee90b16dfaf4ef1f))
* **frontend:** open servers with warnings and sign in from a signed-out view ([#2777](https://github.com/chattocorp/chatto/issues/2777)) ([abed3ba](https://github.com/chattocorp/chatto/commit/abed3ba1a01566f4c8699fd200b0c4448a3b4daf))
* **permissions:** add dedicated message.pin permission ([#2938](https://github.com/chattocorp/chatto/issues/2938)) ([6530141](https://github.com/chattocorp/chatto/commit/65301416c702580ff49acd7bd9e7d5a7bf75f2ca))
* **rooms:** add room invitations and notifications ([#3102](https://github.com/chattocorp/chatto/issues/3102)) ([f000384](https://github.com/chattocorp/chatto/commit/f000384be04ea8e7671b4dd22ed704951251a9a0))
* **runling:** add realtime sources and supervised ChattoBot tasks ([#2527](https://github.com/chattocorp/chatto/issues/2527)) ([7dc4405](https://github.com/chattocorp/chatto/commit/7dc4405447a4beea2a46b9d1c62b9a20fc2e826b))
* **runling:** improve bot task handoffs and activity visibility ([#2537](https://github.com/chattocorp/chatto/issues/2537)) ([216236f](https://github.com/chattocorp/chatto/commit/216236fa6b873f91af623c13f896e16931b67812))
* **threads:** add activity and age sorting and refresh inbox views ([#3013](https://github.com/chattocorp/chatto/issues/3013)) ([feeab41](https://github.com/chattocorp/chatto/commit/feeab41e54646b99cd0151284a9750d44690f31b))
* **threads:** separate followed and own thread views ([#2983](https://github.com/chattocorp/chatto/issues/2983)) ([9455b12](https://github.com/chattocorp/chatto/commit/9455b12090f8197b2306c9de00ad36f014049263))
* **uploads:** use one size limit for all attachments ([#2958](https://github.com/chattocorp/chatto/issues/2958)) ([b9ed12d](https://github.com/chattocorp/chatto/commit/b9ed12d940c0612e4907cd4e444962d650cc7858))


### Bug Fixes

* **auth:** align password validation with bcrypt byte limits ([#3079](https://github.com/chattocorp/chatto/issues/3079)) ([33e31ef](https://github.com/chattocorp/chatto/commit/33e31ef0cf29b4c40e665968a24c55de20e0eb76))
* **auth:** recover bearer sessions after lost refresh responses ([#2941](https://github.com/chattocorp/chatto/issues/2941)) ([93e76ee](https://github.com/chattocorp/chatto/commit/93e76ee346b6d80ad06dab2c43338230db9dad29))
* **client:** preserve reconciliation errors across reconnects ([#3020](https://github.com/chattocorp/chatto/issues/3020)) ([e241bf4](https://github.com/chattocorp/chatto/commit/e241bf4c686bc3136128976a83e24ce92ba91af0))
* **client:** refresh active calls when room membership changes ([#2902](https://github.com/chattocorp/chatto/issues/2902)) ([c1dc255](https://github.com/chattocorp/chatto/commit/c1dc255755a56bdc0fce1e0264409cc1a7ba0982))
* **client:** refresh thread summaries after reply retraction ([#3132](https://github.com/chattocorp/chatto/issues/3132)) ([e6b76b2](https://github.com/chattocorp/chatto/commit/e6b76b241d2dcf23beacb5c1251daaea755041f1))
* **client:** resolve thread reply previews and cache unavailable targets ([#2899](https://github.com/chattocorp/chatto/issues/2899)) ([8190e03](https://github.com/chattocorp/chatto/commit/8190e03aea00338016ab079104af01ac9d4415e5))
* **dev:** keep Authling state per port, reset it, and document client placement ([#2764](https://github.com/chattocorp/chatto/issues/2764)) ([d49c2b5](https://github.com/chattocorp/chatto/commit/d49c2b5c32f909f34a86f1bf857ea58616defe64))
* **frontend:** load previews for linked thread replies ([#3011](https://github.com/chattocorp/chatto/issues/3011)) ([8718c6b](https://github.com/chattocorp/chatto/commit/8718c6bdbcc6e6fcc387e4ed4569ea374641a5f0))
* **frontend:** remove stale My Threads unread dot ([#2775](https://github.com/chattocorp/chatto/issues/2775)) ([8eb4cbe](https://github.com/chattocorp/chatto/commit/8eb4cbecb3b43139cbec07e237217bff1bf68340))
* **messages:** separate posting from read state ([#2868](https://github.com/chattocorp/chatto/issues/2868)) ([3531551](https://github.com/chattocorp/chatto/commit/35315515ea936e729b55386566810ee4dd3bde87))
* **permissions:** load complete permission matrices ([#2966](https://github.com/chattocorp/chatto/issues/2966)) ([5ffdcd2](https://github.com/chattocorp/chatto/commit/5ffdcd2f7f79e0302219376c59741027b9814c89))
* **privileged-mode:** limit permission refresh on resumed reconnects ([#2849](https://github.com/chattocorp/chatto/issues/2849)) ([de0efa2](https://github.com/chattocorp/chatto/commit/de0efa26483904122d8264175b56b4e3df71d80b))
