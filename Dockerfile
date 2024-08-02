# This Dockerfile performs a multi-stage build. BUILDER_IMAGE is the image used
# to compile the celestia-appd binary. RUNTIME_IMAGE is the image that will be
# returned with the final celestia-appd binary.
#
# Separating the builder and runtime image allows the runtime image to be
# considerably smaller because it doesn't need to have Golang installed.
ARG BUILDER_IMAGE=docker.io/golang:1.22.5-alpine3.20
ARG RUNTIME_IMAGE=docker.io/alpine:3.20
ARG TARGETOS
ARG TARGETARCH

# Stage 1: Build the celestia-appd binary inside a builder image that will be discarded later.
# Ignore hadolint rule because hadolint can't parse the variable.
# See https://github.com/hadolint/hadolint/issues/339
# hadolint ignore=DL3006
FROM --platform=$BUILDPLATFORM ${BUILDER_IMAGE} AS builder
ENV CGO_ENABLED=0
ENV GO111MODULE=on
# hadolint ignore=DL3018
RUN apk update && apk add --no-cache \
    gcc \
    git \
    bash \
    # linux-headers are needed for Ledger support
    linux-headers \
    make \
    musl-dev

# Install celestia node & key management CLI for easier debugging and advanced commands
RUN git clone https://github.com/celestiaorg/celestia-node.git

WORKDIR celestia-node

RUN make install-key
RUN make build && make go-install

# Copy all the files manually because otherwise modifying run scripts will trigger a full rebuild and that's annoying
COPY cmd /apollo/cmd
COPY faucet /apollo/faucet
COPY genesis /apollo/genesis
COPY node /apollo/node
COPY web /apollo/web
COPY apollo.go /apollo
COPY conductor.go /apollo
COPY go.mod /apollo
COPY go.sum /apollo
COPY service.go /apollo

WORKDIR /apollo
RUN uname -a &&\
    CGO_ENABLED=${CGO_ENABLED} GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go install cmd/main.go


FROM ${RUNTIME_IMAGE} AS runtime
# Use UID 10,001 because UIDs below 10,000 are a security risk.
# Ref: https://github.com/hexops/dockerfile/blob/main/README.md#do-not-use-a-uid-below-10000
ARG UID=10001
ARG USER_NAME=apollo
ENV APOLLO_HOME=/home/${USER_NAME}

# hadolint ignore=DL3018
RUN apk update && apk add --no-cache \
    bash \
    curl \
    jq \
    && adduser ${USER_NAME} \
    -D \
    -g ${USER_NAME} \
    -h ${APOLLO_HOME} \
    -s /sbin/nologin \
    -u ${UID}

COPY --from=builder /go/bin/main /bin/apollo
COPY --from=builder /go/bin/cel-key /bin/cel-key
COPY --from=builder /go/bin/celestia /bin/celestia

COPY scripts scripts

RUN chmod +x scripts/fund_account.sh
RUN chmod +x scripts/run.sh

# Set the user
USER ${USER_NAME}

# Expose ports:
EXPOSE 1317 9090 26657 1095 8080 26658
ENTRYPOINT [ "/bin/sh", "-c", "scripts/run.sh" ]
