# Changelog

## 0.1.0-alpha.1 (2026-10-10)


### ⚠ BREAKING CHANGES

* **events:** collapse ProjectorStatus failure fields and renew leases through jetstreamutil ([#2839](https://github.com/chattocorp/chatto/issues/2839))
* **events:** move JetStream client helpers into pkg/jetstreamutil ([#2832](https://github.com/chattocorp/chatto/issues/2832))
* **events:** stop guarding writes that do not depend on state ([#2826](https://github.com/chattocorp/chatto/issues/2826))
* **events:** one OCC guard type for every write ([#2825](https://github.com/chattocorp/chatto/issues/2825))
* **events:** slog logging, projector options at construction, one constructor rule ([#2824](https://github.com/chattocorp/chatto/issues/2824))
* **events:** unify projection snapshots and simplify the projector ([#2823](https://github.com/chattocorp/chatto/issues/2823))
* **events:** complete projector startup inside the apply barrier and remove unused API ([#2819](https://github.com/chattocorp/chatto/issues/2819))

### Features

* **events:** extract typed ID interning for projections ([#2864](https://github.com/chattocorp/chatto/issues/2864)) ([8bd9dbf](https://github.com/chattocorp/chatto/commit/8bd9dbf68954754e640b800658239714b9ec2651))


### Bug Fixes

* **auth:** read NATS key-value buckets through the stream leader ([#2804](https://github.com/chattocorp/chatto/issues/2804)) ([54c19db](https://github.com/chattocorp/chatto/commit/54c19db2adbb91eecb78e2c8a45771d36b4d25e5))
* **dev:** keep Authling state per port, reset it, and document client placement ([#2764](https://github.com/chattocorp/chatto/issues/2764)) ([d49c2b5](https://github.com/chattocorp/chatto/commit/d49c2b5c32f909f34a86f1bf857ea58616defe64))
* **events:** avoid races on shared stream metadata ([#2895](https://github.com/chattocorp/chatto/issues/2895)) ([d3e53e8](https://github.com/chattocorp/chatto/commit/d3e53e8d4f1a9bedbbcc3428a91c9570f8571985))
* **events:** complete projector startup inside the apply barrier and remove unused API ([#2819](https://github.com/chattocorp/chatto/issues/2819)) ([52d7e39](https://github.com/chattocorp/chatto/commit/52d7e39bcde75c2103878747faca463f76c281ec))
* **events:** complete startup after target deletion and page byte-bounded reads ([#2813](https://github.com/chattocorp/chatto/issues/2813)) ([ba3faa7](https://github.com/chattocorp/chatto/commit/ba3faa746599abbf4951f29e4b942f1a1914771e))
* **events:** retry JetStream provisioning request timeouts ([#2436](https://github.com/chattocorp/chatto/issues/2436)) ([91e6330](https://github.com/chattocorp/chatto/commit/91e6330f873d9be0c6680b58b89413b4cb3b4b48))
* **logging:** clarify startup outcomes and report server version ([#2885](https://github.com/chattocorp/chatto/issues/2885)) ([b61d683](https://github.com/chattocorp/chatto/commit/b61d6839c586bdc3bbc17814b9a0bfd0f52f5b48))


### Performance Improvements

* **events:** stop guarding writes that do not depend on state ([#2826](https://github.com/chattocorp/chatto/issues/2826)) ([11acb9c](https://github.com/chattocorp/chatto/commit/11acb9c4967180c74a4df48654972e362269a595))
* **realtime:** parallelize NATS lookups on WebSocket connect ([#2646](https://github.com/chattocorp/chatto/issues/2646)) ([7cdffe5](https://github.com/chattocorp/chatto/commit/7cdffe54ee88fd726f472d7441d80e745c74832a))


### Code Refactoring

* **events:** collapse ProjectorStatus failure fields and renew leases through jetstreamutil ([#2839](https://github.com/chattocorp/chatto/issues/2839)) ([6b48808](https://github.com/chattocorp/chatto/commit/6b48808b370082ad29fdf0c5fd8ecab884721e2b))
* **events:** move JetStream client helpers into pkg/jetstreamutil ([#2832](https://github.com/chattocorp/chatto/issues/2832)) ([8bed0d5](https://github.com/chattocorp/chatto/commit/8bed0d59145ebb30d592446f4ccb0dc1c854b10b))
* **events:** one OCC guard type for every write ([#2825](https://github.com/chattocorp/chatto/issues/2825)) ([6a64332](https://github.com/chattocorp/chatto/commit/6a6433258fc7e2a23f3c7251213eda6b7d5033cd))
* **events:** slog logging, projector options at construction, one constructor rule ([#2824](https://github.com/chattocorp/chatto/issues/2824)) ([bf065a9](https://github.com/chattocorp/chatto/commit/bf065a9ff8c8f3493021b2e03672f1549f5d193d))
* **events:** unify projection snapshots and simplify the projector ([#2823](https://github.com/chattocorp/chatto/issues/2823)) ([93ef94c](https://github.com/chattocorp/chatto/commit/93ef94cb65017cc254eeef12d68c07e107a92fbd))

## Changelog

All notable changes to the events framework. Maintained by release-please from
the conventional-commit messages on `main` — do not edit by hand.
