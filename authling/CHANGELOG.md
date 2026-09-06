# Changelog

## 0.1.0-alpha.1 (2026-09-06)


### ⚠ BREAKING CHANGES

* **authling:** limit Authling to identity provider ([#2044](https://github.com/chattocorp/chatto/issues/2044))

### Features

* **appconfig:** extract shared configuration loading ([#1856](https://github.com/chattocorp/chatto/issues/1856)) ([732d7b2](https://github.com/chattocorp/chatto/commit/732d7b22aab7dff63cb4345a5a3e8893d41b57bf))
* **auth:** add CIMD-enabled development stack ([#1881](https://github.com/chattocorp/chatto/issues/1881)) ([5e0f471](https://github.com/chattocorp/chatto/commit/5e0f471459141152ff2130757b120c597a31119b))
* **authling:** add auditable password reset recovery ([#2056](https://github.com/chattocorp/chatto/issues/2056)) ([5d1936f](https://github.com/chattocorp/chatto/commit/5d1936fc93a152d072606613e0077538f7e49b77))
* **authling:** add browser session management ([#2072](https://github.com/chattocorp/chatto/issues/2072)) ([cbf69db](https://github.com/chattocorp/chatto/commit/cbf69db0b1abfdbe17e611161637fc1170d802af))
* **authling:** add CIMD-native OIDC provider ([#1875](https://github.com/chattocorp/chatto/issues/1875)) ([a026593](https://github.com/chattocorp/chatto/commit/a0265939916537d34026964271bd24a6dd6cf51d))
* **authling:** add durable OIDC authorization grants ([#2074](https://github.com/chattocorp/chatto/issues/2074)) ([1e5a016](https://github.com/chattocorp/chatto/commit/1e5a016acf9c3a60f4d9ba7a16c61827e89b7dcd))
* **authling:** add local login and browser sessions ([#1872](https://github.com/chattocorp/chatto/issues/1872)) ([65cf095](https://github.com/chattocorp/chatto/commit/65cf09557d7faac4a211446c77984cd366be2cf1))
* **authling:** add OIDC-authorized account data sync ([#1886](https://github.com/chattocorp/chatto/issues/1886)) ([4953bf1](https://github.com/chattocorp/chatto/commit/4953bf1a2dd5452b273e85684ecbb3ebd35c00ea))
* **authling:** add server-rendered web foundation ([#1861](https://github.com/chattocorp/chatto/issues/1861)) ([12c8957](https://github.com/chattocorp/chatto/commit/12c89579e7b4c18d16e389454f720d8ea1c33e32))
* **authling:** add signed-in password change ([#2059](https://github.com/chattocorp/chatto/issues/2059)) ([dab6594](https://github.com/chattocorp/chatto/commit/dab65944659f06937358f2070cfb3cfa3f7aab97))
* **authling:** add standalone account runtime ([#1842](https://github.com/chattocorp/chatto/issues/1842)) ([e563d68](https://github.com/chattocorp/chatto/commit/e563d681e9a6452c2529e070bd65056a3f6c3a25))
* **authling:** add verified email changes ([#2058](https://github.com/chattocorp/chatto/issues/2058)) ([216a1f8](https://github.com/chattocorp/chatto/commit/216a1f81a920c7ccaee2792ff8ff11c0c7a5f2f2))
* **authling:** add verified email signup ([#1866](https://github.com/chattocorp/chatto/issues/1866)) ([30fef96](https://github.com/chattocorp/chatto/commit/30fef964fe94151f597d7457aef1e5f8d57488c3))
* **authling:** incubate standalone identity provider ([#1828](https://github.com/chattocorp/chatto/issues/1828)) ([93b4637](https://github.com/chattocorp/chatto/commit/93b4637ee498bb45c19399a9b28360b4851aece4))
* **authling:** rotate OIDC signing keys automatically ([#2079](https://github.com/chattocorp/chatto/issues/2079)) ([8d305cd](https://github.com/chattocorp/chatto/commit/8d305cd639694c52a69ea79ddd38d81d9579e3ea))
* **authling:** show signed-in account in site header ([#2071](https://github.com/chattocorp/chatto/issues/2071)) ([aa87c47](https://github.com/chattocorp/chatto/commit/aa87c47489ee16ffb4815a04183bc457e6bc180d))
* **authling:** split verification codes into boxes ([#2062](https://github.com/chattocorp/chatto/issues/2062)) ([0847b2f](https://github.com/chattocorp/chatto/commit/0847b2f9f6857a30bcd65a23b16ef9c0c917223f))
* **auth:** transfer Authling profiles to Chatto ([#2076](https://github.com/chattocorp/chatto/issues/2076)) ([c91cac7](https://github.com/chattocorp/chatto/commit/c91cac7ecb880669f14a24a261c38f0bfcf035d5))
* **datacrypto:** extract shared encryption primitives ([#1853](https://github.com/chattocorp/chatto/issues/1853)) ([4235d7c](https://github.com/chattocorp/chatto/commit/4235d7c0b2fc68ca4393db9cc03d0b88d23c404f))
* **dev:** replace Compose stack with Pitchfork ([#2047](https://github.com/chattocorp/chatto/issues/2047)) ([41f4424](https://github.com/chattocorp/chatto/commit/41f4424fa08a884d0637c90f6c840fef5c9b1900))
* **events:** extract shared framework module ([#1833](https://github.com/chattocorp/chatto/issues/1833)) ([3195ffd](https://github.com/chattocorp/chatto/commit/3195ffd57336dc95196c6d539e0f13aeee087376))
* **natsruntime:** extract embedded NATS lifecycle ([#1845](https://github.com/chattocorp/chatto/issues/1845)) ([cb24279](https://github.com/chattocorp/chatto/commit/cb2427958ec1621d7307a43f72f72f98e7332ebd))


### Bug Fixes

* **authling:** avoid caching failed JWKS responses ([#2080](https://github.com/chattocorp/chatto/issues/2080)) ([fe1d6ca](https://github.com/chattocorp/chatto/commit/fe1d6ca9c3d960f570e594dd3b3ed21afc5c9c2e))
* **authling:** disable 1Password for code inputs ([#2077](https://github.com/chattocorp/chatto/issues/2077)) ([fff3e44](https://github.com/chattocorp/chatto/commit/fff3e445e4f5b3e5188c1625666f795a6dee40e0))
* **authling:** fence credential audit events ([#2083](https://github.com/chattocorp/chatto/issues/2083)) ([19e7edc](https://github.com/chattocorp/chatto/commit/19e7edcbfa9c12db6b5f80e0d7b48ff07d72ef8a))
* **authling:** harden provider edge cases ([#2082](https://github.com/chattocorp/chatto/issues/2082)) ([3ec6073](https://github.com/chattocorp/chatto/commit/3ec6073135a67a6d28a69901bd91125620873407))
* **authling:** require password confirmation during signup ([#2075](https://github.com/chattocorp/chatto/issues/2075)) ([7ac09c3](https://github.com/chattocorp/chatto/commit/7ac09c34b74f1044012fcd890c1eddf6d01c6174))
* **email:** omit unnecessary SMTPUTF8 ([#2273](https://github.com/chattocorp/chatto/issues/2273)) ([0595d00](https://github.com/chattocorp/chatto/commit/0595d0064dc51b0555ac5cfae1cbe7297929678c))


### Code Refactoring

* **authling:** limit Authling to identity provider ([#2044](https://github.com/chattocorp/chatto/issues/2044)) ([ec00d62](https://github.com/chattocorp/chatto/commit/ec00d629f467d1d87c342b929a398db3af26c148))

## Changelog

All notable changes to Authling. Maintained by release-please from the
conventional-commit messages on `main` — do not edit by hand.
