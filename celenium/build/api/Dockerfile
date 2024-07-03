# ---------------------------------------------------------------------
#  The first stage container, for building the application
# ---------------------------------------------------------------------
FROM golang:1.22.4-alpine as builder

ENV CGO_ENABLED=0
ENV GO111MODULE=on
ENV GOOS=linux

RUN apk --no-cache add ca-certificates
RUN apk --no-cache add git

RUN mkdir -p $GOPATH/src/github.com/celenium-io/celestia-indexer/
RUN cd $GOPATH/src/github.com/celenium-io/
RUN git clone https://github.com/celenium-io/celestia-indexer.git $GOPATH/src/github.com/celenium-io/celestia-indexer

WORKDIR $GOPATH/src/github.com/celenium-io/celestia-indexer
RUN go mod download

WORKDIR $GOPATH/src/github.com/celenium-io/celestia-indexer/cmd/api/
RUN go build -a -installsuffix cgo -o /go/bin/api .

WORKDIR $GOPATH/src/github.com/celenium-io/celestia-indexer/

# ---------------------------------------------------------------------
#  The second stage container, for running the application
# ---------------------------------------------------------------------
FROM scratch

WORKDIR /app/api

COPY --from=builder /go/src/github.com/celenium-io/celestia-indexer/configs/dipdup.yml ./
COPY --from=builder /go/src/github.com/celenium-io/celestia-indexer/database database
COPY --from=builder /go/src/github.com/celenium-io/celestia-indexer/cmd/api/docs docs
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /go/bin/api /go/bin/api

ENTRYPOINT ["/go/bin/api", "-c", "dipdup.yml"]
