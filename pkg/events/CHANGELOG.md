# Changelog

## 0.1.0-alpha.1 (2026-08-22)


### ⚠ BREAKING CHANGES

* **notifications:** replace Notifications 1.0 with persistent notifications ([#2020](https://github.com/chattocorp/chatto/issues/2020))

### Features

* **authling:** add server-rendered web foundation ([#1861](https://github.com/chattocorp/chatto/issues/1861)) ([12c8957](https://github.com/chattocorp/chatto/commit/12c89579e7b4c18d16e389454f720d8ea1c33e32))
* **authling:** add standalone account runtime ([#1842](https://github.com/chattocorp/chatto/issues/1842)) ([e563d68](https://github.com/chattocorp/chatto/commit/e563d681e9a6452c2529e070bd65056a3f6c3a25))
* **events:** add durable pull-worker execution ([#1972](https://github.com/chattocorp/chatto/issues/1972)) ([8777064](https://github.com/chattocorp/chatto/commit/877706486ffba9286fc2c5961eb661485b248ffa))
* **events:** add selectable mutation consistency boundaries ([#1962](https://github.com/chattocorp/chatto/issues/1962)) ([3db6575](https://github.com/chattocorp/chatto/commit/3db65758a8b68e4d0e1373d51790e8afd374533a))
* **events:** extract shared framework module ([#1833](https://github.com/chattocorp/chatto/issues/1833)) ([3195ffd](https://github.com/chattocorp/chatto/commit/3195ffd57336dc95196c6d539e0f13aeee087376))
* **natsruntime:** extract embedded NATS lifecycle ([#1845](https://github.com/chattocorp/chatto/issues/1845)) ([cb24279](https://github.com/chattocorp/chatto/commit/cb2427958ec1621d7307a43f72f72f98e7332ebd))
* **notifications:** replace Notifications 1.0 with persistent notifications ([#2020](https://github.com/chattocorp/chatto/issues/2020)) ([29c88d9](https://github.com/chattocorp/chatto/commit/29c88d98d495f886fefd8c64948a0826293a066f))


### Bug Fixes

* **events:** add bounded subject reads ([#2002](https://github.com/chattocorp/chatto/issues/2002)) ([1a15202](https://github.com/chattocorp/chatto/commit/1a152024bc07a0483fb194f920c1ddf738714272))
* **events:** guard single-run lifecycles ([#2003](https://github.com/chattocorp/chatto/issues/2003)) ([e4f60f3](https://github.com/chattocorp/chatto/commit/e4f60f3b25252ce8da826ff1f1dd04bfc027c668))
* **events:** make nil loggers safe ([#1999](https://github.com/chattocorp/chatto/issues/1999)) ([2b52ded](https://github.com/chattocorp/chatto/commit/2b52ded76772168f9ba6da2417552152977be957))
* **events:** require pointer projections ([#2000](https://github.com/chattocorp/chatto/issues/2000)) ([4ee0c2b](https://github.com/chattocorp/chatto/commit/4ee0c2b1bb8c7ffff7c1aca0c76acba848fb2e70))
* **events:** validate snapshot bindings ([#2001](https://github.com/chattocorp/chatto/issues/2001)) ([d323298](https://github.com/chattocorp/chatto/commit/d323298eafb1a056fd736903beaf79bb0514c897))
* **workers:** harden durable recovery ([#1978](https://github.com/chattocorp/chatto/issues/1978)) ([c1c6f3e](https://github.com/chattocorp/chatto/commit/c1c6f3e35510da1e19cfdfcc30ed060494600f05))

## Changelog

All notable changes to the events framework. Maintained by release-please from
the conventional-commit messages on `main` — do not edit by hand.
