MODULE ?= github.com/isometry/platform-health
export KO_DOCKER_REPO ?= ghcr.io/isometry/platform-health

.PHONY: build
build: protoc
	goreleaser build --clean --single-target --snapshot --skip=post-hooks

.PHONY: ko-build
ko-build: protoc generate
	ko build --bare ./cmd/server

.PHONY: generate
generate:
	go generate ./...

protoc: pkg/platform_health/platform_health.pb.go pkg/platform_health/platform_health_grpc.pb.go pkg/platform_health/details/detail_loop.pb.go pkg/platform_health/details/detail_tls.pb.go

pkg/platform_health/platform_health.pb.go: proto/platform_health.proto
	protoc --go_out=. --go_opt=module=$(MODULE)  $<
pkg/platform_health/platform_health_grpc.pb.go: proto/platform_health.proto
	protoc --go-grpc_out=. --go-grpc_opt=module=$(MODULE) $<
pkg/platform_health/details/detail_tls.pb.go: proto/detail_tls.proto
	protoc --go_out=. --go_opt=module=$(MODULE)  $<
pkg/platform_health/details/detail_loop.pb.go: proto/detail_loop.proto
	protoc --go_out=. --go_opt=module=$(MODULE)  $<
