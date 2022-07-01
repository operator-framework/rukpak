FROM gcr.io/distroless/static:debug
WORKDIR /

COPY provisioner provisioner
COPY unpack unpack
COPY core core
COPY crdvalidator crdvalidator

EXPOSE 8080
