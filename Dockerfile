FROM gcr.io/distroless/static:debug
WORKDIR /

COPY plain plain
COPY unpack unpack
COPY core core
COPY crdvalidator crdvalidator

EXPOSE 8080
ENTRYPOINT ["/plain"]
