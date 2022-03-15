FROM golang:1.17-buster AS builder

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

COPY Makefile Makefile
COPY cmd cmd
COPY api api
COPY internal internal
# copy git-related information for binary version information
COPY .git/HEAD .git/HEAD
COPY .git/refs/heads/. .git/refs/heads
RUN mkdir -p .git/objects

RUN make build

FROM gcr.io/distroless/static:debug

WORKDIR /
COPY --from=builder /workspace/bin/plain .
COPY --from=builder /workspace/bin/unpack .
COPY --from=builder /workspace/bin/core .
EXPOSE 8080

ENTRYPOINT ["/plain"]
CMD ["run"]
