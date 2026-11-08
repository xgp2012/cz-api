# syntax=docker/dockerfile:1.7
#
# Build cost model, in the order the layers are evaluated:
#
#   1. `web_deps`      -> `bun install`; invalidated only by web/package.json,
#                         web/bun.lock and web/.npmrc. Output lives in a BuildKit
#                         cache mount, so a source change reuses the installed
#                         tree instead of re-downloading ~1.5 GB of packages.
#   2. `build_web`     -> Rsbuild; invalidated by any change under web/.
#   3. `build_backend` -> `go build`; invalidated by any change in the Go
#                         sources, which is by far the most common edit.
#
# The frontend and the backend therefore run concurrently, and neither stage
# downloads the other toolchain dependencies. `go mod download` and the
# compiler cache are BuildKit cache mounts too, so a one-line Go change reruns
# only `go build` against already-downloaded modules and already-compiled
# packages.
#
# Keep every COPY that can invalidate a stage narrower than the bare context.
# A repository-root edit (README, docs, workflows) must not invalidate the
# dependency layers.

ARG BUN_IMAGE=oven/bun:1.4.4@sha256:1d22548f177f4ff38e0c15baa3c2c46c6c2060ba2d1bc573bb4720b407548f1d
ARG GO_IMAGE=golang:1.26.10-alpine@sha256:9ce231b28998b32e116927c61aa5c406183061d4f3f2d61b292c88e7f6bdf7ca
ARG RUNTIME_IMAGE=debian:bookworm-slim@sha256:f06537653ac770703bc45b4b113475bd402f451e85223f0f2837acbf89ab020a

# ---------------------------------------------------------------------------
# Frontend dependencies
# ---------------------------------------------------------------------------
FROM ${BUN_IMAGE} AS web_deps

WORKDIR /build/web
# Only the files that can change the resolved dependency tree. Do not add
# source files here: that would invalidate `bun install` on every frontend edit.
COPY web/package.json web/bun.lock ./
COPY web/.npmrc ./
RUN --mount=type=cache,id=bun-install,target=/root/.bun/install/cache,sharing=locked \
    bun install --frozen-lockfile

# ---------------------------------------------------------------------------
# Frontend build
# ---------------------------------------------------------------------------
FROM web_deps AS build_web

# Invalidate this stage whenever frontend sources change, and keep the
# dependency tree intact in the same layer commit.
COPY web/ ./
COPY ./VERSION /build/VERSION

ENV NODE_ENV=production
RUN DISABLE_ESLINT_PLUGIN='true' \
    VITE_REACT_APP_VERSION="$(cat /build/VERSION)" \
    bun run build

# ---------------------------------------------------------------------------
# Backend build
# ---------------------------------------------------------------------------
FROM ${GO_IMAGE} AS build_backend

# The two submodules are wired in through `replace`; GOWORK=off keeps the root
# module independent of any workspace file.
ENV GO111MODULE=on CGO_ENABLED=0 GOWORK=off
# Build-time module metadata lookups need a registry the build can reach. When
# they miss the local module cache and the default endpoints are unreachable,
# `go list -m` prints nothing and the failure surfaces as "preprofile: error
# opening profile: open /default.pgo: no such file or directory". The fallback
# chain keeps a reachable default working.
ENV GOPROXY=https://goproxy.cn,https://proxy.golang.org,direct

ARG TARGETOS
ARG TARGETARCH
ENV GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64}
ENV GOEXPERIMENT=greenteagc

WORKDIR /build

# Only the module manifests, so the dependency graph resolves from cache while
# the source tree changes underneath.
COPY go.mod go.sum ./
# relaykit and tokenkit are local submodules referenced via replace; their
# go.mod files must be present for go mod download to resolve the main module graph.
COPY relaykit/go.mod ./relaykit/go.mod
COPY tokenkit/go.mod ./tokenkit/go.mod
RUN --mount=type=cache,id=go-mod,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,id=go-build,target=/root/.cache/go-build,sharing=locked \
    go mod download

# Go sources only. `web/dist` and the licence files are copied afterwards so an
# image rebuild after a frontend-only change does not recompile the host. The
# context includes web/, whose install and build trees are excluded by
# .dockerignore.
COPY . .
COPY --from=build_web /build/web/dist ./web/dist

RUN --mount=type=cache,id=go-mod,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,id=go-build,target=/root/.cache/go-build,sharing=locked \
    BUILD_VERSION="$(cat VERSION)" && \
    MOEJS_PGO="$(go list -m -f '{{.Dir}}' github.com/Calcium-Ion/moejs)/default.pgo" && \
    go build -pgo="${MOEJS_PGO}" \
        -trimpath \
        -ldflags "-s -w -X 'github.com/QuantumNous/new-api/common.Version=${BUILD_VERSION}'" \
        -o new-api

# ---------------------------------------------------------------------------
# Runtime
# ---------------------------------------------------------------------------
FROM ${RUNTIME_IMAGE}

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates tzdata libasan8 wget \
    && rm -rf /var/lib/apt/lists/* \
    && update-ca-certificates

COPY --from=build_backend /build/new-api /
COPY LICENSE NOTICE THIRD-PARTY-LICENSES.md /licenses/
EXPOSE 3000
WORKDIR /data
ENTRYPOINT ["/new-api"]
