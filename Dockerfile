FROM gcr.io/distroless/static:debug
WORKDIR /

COPY plain plain
COPY registry registry
COPY unpack unpack
COPY core core
COPY binarymgr binarymgr
COPY crdvalidator crdvalidator

EXPOSE 8080
