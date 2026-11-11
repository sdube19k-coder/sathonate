# Build context contains a Linux fabric-api binary built from this repository.
# Supply a pinned customer-approved runtime base with a system CA trust store.
ARG RUNTIME_IMAGE
FROM ${RUNTIME_IMAGE}
COPY fabric-api /fabric-api
USER 10001:10001
ENTRYPOINT ["/fabric-api"]
