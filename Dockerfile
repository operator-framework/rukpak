FROM gcr.io/distroless/static:debug-nonroot AS builder

# Stage 2: 
FROM gcr.io/distroless/static:nonroot

# Grab the cp binary so we can cp the unpack
# binary to a shared volume in the bundle image.
COPY --from=builder /busybox/cp /busybox/ls /

WORKDIR /

COPY helm helm
COPY core core
COPY unpack unpack
COPY webhooks webhooks
COPY crdvalidator crdvalidator

EXPOSE 8080
