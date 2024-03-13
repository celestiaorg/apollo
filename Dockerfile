FROM golang:1.22 AS builder

WORKDIR /app

# Copy the local package files to the container's workspace.
COPY . .

# Build the apollo command inside the container.
# (Assuming that the main package is located in the "cmd/apollo.go" directory)
RUN go build -o /apollo cmd/apollo.go

# Use a Docker multi-stage build to create a lean production image.
FROM debian:buster-slim

# Copy the binary to the production image from the builder stage.
COPY --from=builder /apollo /apollo

EXPOSE 26657 1317 9090 26658

# Run the apollo command by default when the container starts.
ENTRYPOINT ["/apollo"]

