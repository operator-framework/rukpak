FROM gcr.io/distroless/static:debug
WORKDIR /

COPY plain plain
COPY unpack unpack
COPY core core

EXPOSE 8080
ENTRYPOINT ["/plain"]
