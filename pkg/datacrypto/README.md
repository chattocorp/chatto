# Data Cryptography

`hmans.de/chatto/pkg/datacrypto` provides small, application-neutral
cryptographic primitives for protected data:

- cryptographically random 256-bit keys;
- XChaCha20-Poly1305 authenticated encryption; and
- authenticated wrapping and unwrapping of 256-bit keys.

Applications retain ownership of associated-data construction, domain
separation, key identities, key hierarchies, storage, KMS integration, caching,
rotation, and erasure policy. The package does not serialize an envelope;
callers persist the returned ciphertext and nonce in their own formats.

## Status

This module is an incubation surface shared by Chatto and Authling. Its API is
not yet covered by a stability promise, and releases remain pre-1.0 while both
applications establish the smallest useful contract.

## Development

Run the module tests independently:

```sh
mise test-datacrypto
```

From this directory, the equivalent standalone check is:

```sh
GOWORK=off go test ./...
```

## License

The module is licensed under [`Apache-2.0`](LICENSE). Its permissive license
does not imply API stability.

