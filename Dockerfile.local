FROM gcr.io/distroless/static:debug

WORKDIR /
COPY bin/plain-linux plain
COPY bin/unpack-linux unpack
EXPOSE 8080

ENTRYPOINT ["/plain"]
CMD ["run"]
