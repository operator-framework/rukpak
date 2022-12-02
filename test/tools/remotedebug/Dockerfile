FROM golang:1.17 as build-env

# Install Delve
RUN CGO_ENABLED=0 go install -ldflags "-s -w -extldflags '-static'" github.com/go-delve/delve/cmd/dlv@latest

FROM gcr.io/distroless/static:debug-nonroot

WORKDIR /

COPY --from=build-env /go/bin/dlv /
COPY helm helm
COPY core core
COPY unpack unpack
COPY webhooks webhooks
COPY crdvalidator crdvalidator
COPY rukpakctl rukpakctl

EXPOSE 8080
