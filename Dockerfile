FROM gcr.io/distroless/static:debug-nonroot

WORKDIR /

COPY helm helm
COPY core core
COPY unpack unpack
COPY webhooks webhooks
COPY crdvalidator crdvalidator
COPY rukpakctl rukpakctl
COPY kustomize kustomize

EXPOSE 8080
