# syntax = docker/dockerfile-upstream:1.10.0-labs

# THIS FILE WAS AUTOMATICALLY GENERATED, PLEASE DO NOT EDIT.
#
# Generated on 2024-10-10T10:53:36Z by kres 34e72ac.

ARG TOOLCHAIN

FROM ghcr.io/siderolabs/ca-certificates:v1.8.0 AS image-ca-certificates

FROM ghcr.io/siderolabs/fhs:v1.8.0 AS image-fhs

# runs markdownlint
FROM docker.io/oven/bun:1.1.29-alpine AS lint-markdown
WORKDIR /src
RUN bun i markdownlint-cli@0.41.0 sentences-per-line@0.2.1
COPY .markdownlint.json .
COPY ./README.md ./README.md
RUN bunx markdownlint --ignore "CHANGELOG.md" --ignore "**/node_modules/**" --ignore '**/hack/chglog/**' --rules node_modules/sentences-per-line/index.js .

# collects proto specs
FROM scratch AS proto-specs
ADD api/specs/specs.proto /api/specs/
ADD api/agent/agent.proto /api/agent/

# base toolchain image
FROM --platform=${BUILDPLATFORM} ${TOOLCHAIN} AS toolchain
RUN apk --update --no-cache add bash curl build-base protoc protobuf-dev

# build tools
FROM --platform=${BUILDPLATFORM} toolchain AS tools
ENV GO111MODULE=on
ARG CGO_ENABLED
ENV CGO_ENABLED=${CGO_ENABLED}
ARG GOTOOLCHAIN
ENV GOTOOLCHAIN=${GOTOOLCHAIN}
ARG GOEXPERIMENT
ENV GOEXPERIMENT=${GOEXPERIMENT}
ENV GOPATH=/go
ARG GOIMPORTS_VERSION
RUN --mount=type=cache,target=/root/.cache/go-build --mount=type=cache,target=/go/pkg go install golang.org/x/tools/cmd/goimports@v${GOIMPORTS_VERSION}
RUN mv /go/bin/goimports /bin
ARG PROTOBUF_GO_VERSION
RUN --mount=type=cache,target=/root/.cache/go-build --mount=type=cache,target=/go/pkg go install google.golang.org/protobuf/cmd/protoc-gen-go@v${PROTOBUF_GO_VERSION}
RUN mv /go/bin/protoc-gen-go /bin
ARG GRPC_GO_VERSION
RUN --mount=type=cache,target=/root/.cache/go-build --mount=type=cache,target=/go/pkg go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v${GRPC_GO_VERSION}
RUN mv /go/bin/protoc-gen-go-grpc /bin
ARG GRPC_GATEWAY_VERSION
RUN --mount=type=cache,target=/root/.cache/go-build --mount=type=cache,target=/go/pkg go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@v${GRPC_GATEWAY_VERSION}
RUN mv /go/bin/protoc-gen-grpc-gateway /bin
ARG VTPROTOBUF_VERSION
RUN --mount=type=cache,target=/root/.cache/go-build --mount=type=cache,target=/go/pkg go install github.com/planetscale/vtprotobuf/cmd/protoc-gen-go-vtproto@v${VTPROTOBUF_VERSION}
RUN mv /go/bin/protoc-gen-go-vtproto /bin
ARG DEEPCOPY_VERSION
RUN --mount=type=cache,target=/root/.cache/go-build --mount=type=cache,target=/go/pkg go install github.com/siderolabs/deep-copy@${DEEPCOPY_VERSION} \
	&& mv /go/bin/deep-copy /bin/deep-copy
ARG GOLANGCILINT_VERSION
RUN --mount=type=cache,target=/root/.cache/go-build --mount=type=cache,target=/go/pkg go install github.com/golangci/golangci-lint/cmd/golangci-lint@${GOLANGCILINT_VERSION} \
	&& mv /go/bin/golangci-lint /bin/golangci-lint
RUN --mount=type=cache,target=/root/.cache/go-build --mount=type=cache,target=/go/pkg go install golang.org/x/vuln/cmd/govulncheck@latest \
	&& mv /go/bin/govulncheck /bin/govulncheck
ARG GOFUMPT_VERSION
RUN go install mvdan.cc/gofumpt@${GOFUMPT_VERSION} \
	&& mv /go/bin/gofumpt /bin/gofumpt

# tools and sources
FROM tools AS base
WORKDIR /src
COPY go.mod go.mod
COPY go.sum go.sum
RUN cd .
RUN --mount=type=cache,target=/go/pkg go mod download
RUN --mount=type=cache,target=/go/pkg go mod verify
COPY ./api ./api
COPY ./cmd ./cmd
COPY ./internal ./internal
RUN --mount=type=cache,target=/go/pkg go list -mod=readonly all >/dev/null

FROM tools AS embed-generate
ARG SHA
ARG TAG
WORKDIR /src
RUN mkdir -p internal/version/data && \
    echo -n ${SHA} > internal/version/data/sha && \
    echo -n ${TAG} > internal/version/data/tag

# runs protobuf compiler
FROM tools AS proto-compile
COPY --from=proto-specs / /
RUN protoc -I/api --go_out=paths=source_relative:/api --go-grpc_out=paths=source_relative:/api --go-vtproto_out=paths=source_relative:/api --go-vtproto_opt=features=marshal+unmarshal+size+equal+clone /api/specs/specs.proto
RUN protoc -I/api --grpc-gateway_out=paths=source_relative:/api --grpc-gateway_opt=generate_unbound_methods=true --go_out=paths=source_relative:/api --go-grpc_out=paths=source_relative:/api --go-vtproto_out=paths=source_relative:/api --go-vtproto_opt=features=marshal+unmarshal+size+equal+clone /api/agent/agent.proto
RUN rm /api/specs/specs.proto
RUN rm /api/agent/agent.proto
RUN goimports -w -local github.com/siderolabs/omni-infra-provider-bare-metal /api
RUN gofumpt -w /api

# runs gofumpt
FROM base AS lint-gofumpt
RUN FILES="$(gofumpt -l .)" && test -z "${FILES}" || (echo -e "Source code is not formatted with 'gofumpt -w .':\n${FILES}"; exit 1)

# runs golangci-lint
FROM base AS lint-golangci-lint
WORKDIR /src
COPY .golangci.yml .
ENV GOGC=50
RUN golangci-lint config verify --config .golangci.yml
RUN --mount=type=cache,target=/root/.cache/go-build --mount=type=cache,target=/root/.cache/golangci-lint --mount=type=cache,target=/go/pkg golangci-lint run --config .golangci.yml

# runs govulncheck
FROM base AS lint-govulncheck
WORKDIR /src
RUN --mount=type=cache,target=/root/.cache/go-build --mount=type=cache,target=/go/pkg govulncheck ./...

# runs unit-tests with race detector
FROM base AS unit-tests-race
WORKDIR /src
ARG TESTPKGS
RUN --mount=type=cache,target=/root/.cache/go-build --mount=type=cache,target=/go/pkg --mount=type=cache,target=/tmp CGO_ENABLED=1 go test -v -race -count 1 ${TESTPKGS}

# runs unit-tests
FROM base AS unit-tests-run
WORKDIR /src
ARG TESTPKGS
RUN --mount=type=cache,target=/root/.cache/go-build --mount=type=cache,target=/go/pkg --mount=type=cache,target=/tmp go test -v -covermode=atomic -coverprofile=coverage.txt -coverpkg=${TESTPKGS} -count 1 ${TESTPKGS}

FROM embed-generate AS embed-abbrev-generate
WORKDIR /src
ARG ABBREV_TAG
RUN echo -n 'undefined' > internal/version/data/sha && \
    echo -n ${ABBREV_TAG} > internal/version/data/tag

FROM scratch AS unit-tests
COPY --from=unit-tests-run /src/coverage.txt /coverage-unit-tests.txt

# cleaned up specs and compiled versions
FROM scratch AS generate
COPY --from=proto-compile /api/ /api/
COPY --from=embed-abbrev-generate /src/internal/version internal/version

# builds agent-linux-amd64
FROM base AS agent-linux-amd64-build
COPY --from=generate / /
COPY --from=embed-generate / /
WORKDIR /src/cmd/agent
ARG GO_BUILDFLAGS
ARG GO_LDFLAGS
ARG VERSION_PKG="internal/version"
ARG SHA
ARG TAG
RUN --mount=type=cache,target=/root/.cache/go-build --mount=type=cache,target=/go/pkg GOARCH=amd64 GOOS=linux go build ${GO_BUILDFLAGS} -ldflags "${GO_LDFLAGS} -X ${VERSION_PKG}.Name=agent -X ${VERSION_PKG}.SHA=${SHA} -X ${VERSION_PKG}.Tag=${TAG}" -o /agent-linux-amd64

# builds agent-linux-arm64
FROM base AS agent-linux-arm64-build
COPY --from=generate / /
COPY --from=embed-generate / /
WORKDIR /src/cmd/agent
ARG GO_BUILDFLAGS
ARG GO_LDFLAGS
ARG VERSION_PKG="internal/version"
ARG SHA
ARG TAG
RUN --mount=type=cache,target=/root/.cache/go-build --mount=type=cache,target=/go/pkg GOARCH=arm64 GOOS=linux go build ${GO_BUILDFLAGS} -ldflags "${GO_LDFLAGS} -X ${VERSION_PKG}.Name=agent -X ${VERSION_PKG}.SHA=${SHA} -X ${VERSION_PKG}.Tag=${TAG}" -o /agent-linux-arm64

# builds omni-cloud-provider-qemu-linux-amd64
FROM base AS omni-cloud-provider-qemu-linux-amd64-build
COPY --from=generate / /
COPY --from=embed-generate / /
WORKDIR /src/cmd/omni-cloud-provider-qemu
ARG GO_BUILDFLAGS
ARG GO_LDFLAGS
ARG VERSION_PKG="internal/version"
ARG SHA
ARG TAG
RUN --mount=type=cache,target=/root/.cache/go-build --mount=type=cache,target=/go/pkg GOARCH=amd64 GOOS=linux go build ${GO_BUILDFLAGS} -ldflags "${GO_LDFLAGS} -X ${VERSION_PKG}.Name=omni-cloud-provider-qemu -X ${VERSION_PKG}.SHA=${SHA} -X ${VERSION_PKG}.Tag=${TAG}" -o /omni-cloud-provider-qemu-linux-amd64

# builds omni-cloud-provider-qemu-linux-arm64
FROM base AS omni-cloud-provider-qemu-linux-arm64-build
COPY --from=generate / /
COPY --from=embed-generate / /
WORKDIR /src/cmd/omni-cloud-provider-qemu
ARG GO_BUILDFLAGS
ARG GO_LDFLAGS
ARG VERSION_PKG="internal/version"
ARG SHA
ARG TAG
RUN --mount=type=cache,target=/root/.cache/go-build --mount=type=cache,target=/go/pkg GOARCH=arm64 GOOS=linux go build ${GO_BUILDFLAGS} -ldflags "${GO_LDFLAGS} -X ${VERSION_PKG}.Name=omni-cloud-provider-qemu -X ${VERSION_PKG}.SHA=${SHA} -X ${VERSION_PKG}.Tag=${TAG}" -o /omni-cloud-provider-qemu-linux-arm64

FROM scratch AS agent-linux-amd64
COPY --from=agent-linux-amd64-build /agent-linux-amd64 /agent-linux-amd64

FROM scratch AS agent-linux-arm64
COPY --from=agent-linux-arm64-build /agent-linux-arm64 /agent-linux-arm64

FROM scratch AS omni-cloud-provider-qemu-linux-amd64
COPY --from=omni-cloud-provider-qemu-linux-amd64-build /omni-cloud-provider-qemu-linux-amd64 /omni-cloud-provider-qemu-linux-amd64

FROM scratch AS omni-cloud-provider-qemu-linux-arm64
COPY --from=omni-cloud-provider-qemu-linux-arm64-build /omni-cloud-provider-qemu-linux-arm64 /omni-cloud-provider-qemu-linux-arm64

FROM agent-linux-${TARGETARCH} AS agent

FROM scratch AS agent-all
COPY --from=agent-linux-amd64 / /
COPY --from=agent-linux-arm64 / /

FROM omni-cloud-provider-qemu-linux-${TARGETARCH} AS omni-cloud-provider-qemu

FROM scratch AS omni-cloud-provider-qemu-all
COPY --from=omni-cloud-provider-qemu-linux-amd64 / /
COPY --from=omni-cloud-provider-qemu-linux-arm64 / /

FROM scratch AS image-agent
ARG TARGETARCH
COPY --from=agent agent-linux-${TARGETARCH} /agent
COPY --from=image-fhs / /
COPY --from=image-ca-certificates / /
LABEL org.opencontainers.image.source=https://github.com/siderolabs/omni-cloud-provider-qemu
ENTRYPOINT ["/agent"]

FROM scratch AS image-omni-cloud-provider-qemu
ARG TARGETARCH
COPY --from=omni-cloud-provider-qemu omni-cloud-provider-qemu-linux-${TARGETARCH} /omni-cloud-provider-qemu
COPY --from=image-fhs / /
COPY --from=image-ca-certificates / /
LABEL org.opencontainers.image.source=https://github.com/siderolabs/omni-cloud-provider-qemu
ENTRYPOINT ["/omni-cloud-provider-qemu"]

