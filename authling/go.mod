module hmans.de/authling

go 1.26.0

tool google.golang.org/protobuf/cmd/protoc-gen-go

require (
	github.com/caarlos0/env/v11 v11.4.1
	github.com/nats-io/nats-server/v2 v2.14.3
	github.com/nats-io/nats.go v1.52.0
	github.com/pelletier/go-toml/v2 v2.4.3
	google.golang.org/protobuf v1.36.11
	hmans.de/chatto/pkg/events v0.0.0
)

require (
	github.com/antithesishq/antithesis-sdk-go v0.7.2 // indirect
	github.com/google/go-tpm v0.9.8 // indirect
	github.com/klauspost/compress v1.19.1 // indirect
	github.com/minio/highwayhash v1.0.4 // indirect
	github.com/nats-io/jwt/v2 v2.8.2 // indirect
	github.com/nats-io/nkeys v0.4.16 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	golang.org/x/crypto v0.54.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/time v0.15.0 // indirect
)

replace hmans.de/chatto/pkg/events => ../pkg/events
