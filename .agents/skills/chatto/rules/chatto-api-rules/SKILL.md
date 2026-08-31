---
name: "chatto-api-rules"
description: "Design rules concerning Chatto's ConnectRPC API, both resources and realtime"
---

### ConnectRPC Resource API

_TODO_

### Realtime API

- Completeness: The realtime protocol must include/provide the entire product surface (to the extent that it is useful to integrations, bots, clients), even if there are bits and pieces even our own frontend doesn't make use of.
- Wire transfer efficiency: We want to make sure we don't spam the client and transmit needlessly huge amounts of data (even if they're just protobufs.)
- Simplicity: The Realtime API must be easy to use by integrations and clients. For this, it must be sufficiently simple to understand and reason about, be thoroughly consistent, and well-documented.
- Consistency: Consistent and easy to follow naming (and structure) of the involved protobufs. Names and documentation should not make use of Chatto backend implementation or architecture terminology (eg. "projections", NATS, JetStream, ...)
- Documentation: The protobuf documentation as well as the API overviews and tutorials in the documentation website must be complete and up to date.
