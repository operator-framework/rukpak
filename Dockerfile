FROM gcr.io/distroless/static:nonroot
WORKDIR /

COPY helm helm
COPY core core
COPY unpack unpack
COPY webhooks webhooks
COPY crdvalidator crdvalidator
COPY rukpakctl rukpakctl

EXPOSE 8080
