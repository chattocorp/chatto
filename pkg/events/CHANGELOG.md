# Changelog

## 0.2.0

### Features

- Add `TerminationReason`, which reports whether an error came from `TerminateDelivery`. Applications that run a `DurableDeliveryHandler` outside a `DurableWorker` can use it to keep the same non-retryable classification. ([#3598](https://github.com/chattocorp/chatto/pull/3598))
- Detect stalled projectors. `ProjectorStatus` now reports `ObservedSeq`, `LastCommitAt`, `LastCommitDuration`, `CurrentOperation`, `CurrentOperationDuration`, `PendingFor`, and `Stalled`. A projector is stalled when matching events wait and it commits no progress for `ProjectorOptions.StallAfter` (default five minutes). An idle, caught-up projector and a slow replay that still commits are not stalled. `Run` reads the newest matching stream sequence every 30 seconds, so a consumer that stops delivering also shows pending work. `Status` does not wait for the apply barrier. ([#3668](https://github.com/chattocorp/chatto/pull/3668))

### Fixes

- An idle `DurableWorker` now holds one pull request for up to 30 seconds (or the consumer's shorter `MaxRequestExpires`) instead of one for each `FetchMaxWait`, which reduces idle JetStream API traffic by about 30 times. The worker becomes idle only after a pull expires empty while no delivery is active, so a backlog and the shutdown handoff use `FetchMaxWait` pulls as before. A replica that disappears without closing its connection can hold its idle pull for up to about 29 seconds; work that the server assigns to that pull waits for the consumer's `AckWait`. ([#3782](https://github.com/chattocorp/chatto/pull/3782))
- `WaitFor` no longer reads the target message from the stream leader when `LastSeq` already covers the position. A wait that has not ended after 100 ms reads the message once, in parallel with the wait, and still fails for a position that the projector cannot reach. A subject filter that cannot match a consumed subject fails at once without a read. A wrong subject and sequence pair is no longer reported when the projector is already past the position. ([#3780](https://github.com/chattocorp/chatto/pull/3780))
- Atomic batch publish errors preserve the JetStream API error code so callers can identify a temporary rejection. ([#3744](https://github.com/chattocorp/chatto/pull/3744))
- Projectors now apply events in stream order and exactly once across an ordered consumer reset. Before, after a reconnect, missed heartbeats, or a NATS server change, the next event could be applied while the previous event was still in decode, so the projection applied the two events in the wrong order and `WaitFor` for the previous event could return before that event was applied. A projector now ignores a message whose sequence it already handled, fails when its consumer stops before `Run` ends (for example after the connection closes), and `Run` returns only after the message in progress is handled. Startup reconciliation and `WaitForCurrent` also wait for an event that the projector received but has not applied yet, also when that event was deleted from the stream after delivery. Projectors now pull up to 1,000 messages at a time and have no byte limit for each pull, so a projector that is behind can buffer up to 1,000 events of any size. Before, projectors used a client byte window of 16 MiB. ([#3794](https://github.com/chattocorp/chatto/pull/3794))
- Update the NATS Go client to v1.56.0. Projector ordered consumers now report missed heartbeats after about 10 seconds instead of 30, and completed durable worker pulls stop their timers. ([#3790](https://github.com/chattocorp/chatto/pull/3790))

## 0.1.0 (2026-10-15)


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
