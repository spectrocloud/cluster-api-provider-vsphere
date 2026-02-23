# syntax=docker/dockerfile:1.4

# Copyright 2019 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Build the manager binary
ARG BUILDER_GOLANG_VERSION
# First stage: build the executable.
FROM --platform=$TARGETPLATFORM us-docker.pkg.dev/palette-images/build-base-images/golang:${BUILDER_GOLANG_VERSION}-alpine as toolchain
# Run this with docker build --build_arg $(go env GOPROXY) to override the goproxy
ARG goproxy=https://proxy.golang.org
ENV GOPROXY=$goproxy

# FIPS
ARG CRYPTO_LIB
ENV GOEXPERIMENT=${CRYPTO_LIB:+boringcrypto}

FROM toolchain as builder
WORKDIR /workspace

RUN apk update
RUN apk add git gcc g++ curl binutils-gold

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum

# Cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer

RUN go mod download

# Copy the sources
COPY ./ ./

# Build
ARG ARCH
ARG ldflags
RUN if [ ${CRYPTO_LIB} ]; \
    then \
      GOARCH=${ARCH} go-build-fips.sh -a -o manager . ;\
    else \
      GOARCH=${ARCH} go-build-static.sh -a -o manager . ;\
    fi
RUN if [ "${CRYPTO_LIB}" ]; then assert-static.sh manager; fi
RUN if [ "${CRYPTO_LIB}" ]; then assert-fips.sh manager; fi
# Removed for PCP-6049. To be enable later
# RUN scan-govulncheck.sh manager

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/manager .
# Use uid of nonroot user (65532) because kubernetes expects numeric user when applying PSPs
USER 65532
ENTRYPOINT ["/manager"]
