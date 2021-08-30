FROM golang:1.17-buster AS builder

WORKDIR /build
COPY . .

RUN make build

FROM gcr.io/distroless/static:debug

WORKDIR /
COPY --from=builder /build/bin/k8s .

EXPOSE 8080

ENTRYPOINT ["k8s"]
CMD ["run"]
