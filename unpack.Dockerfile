# Build the manager binary
FROM golang:1.17 as builder

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY cmd/unpack/ cmd/unpack

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o unpack ./cmd/unpack/

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
#
# Note(tflannag): Use the `debug` image tag so we have access to a shell when unpacking
# Bundle contents. This allows us to use the `cp` shell command for copying the /unpack binary
# to a scratch-based container image, which doesn't contain a shell environment, but contains the
# Bundle manifests we need to extract.
FROM gcr.io/distroless/static:debug
WORKDIR /
COPY --from=builder /workspace/unpack .
USER 65532:65532

ENTRYPOINT ["/unpack"]
