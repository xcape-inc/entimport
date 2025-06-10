ARG BASE_IMAGE=alpine
ARG BASE_IMAGE_VERSION=3.16
FROM ${BASE_IMAGE}:${BASE_IMAGE_VERSION}
ARG BINARY=entimport
RUN mkdir -p /opt/"${BINARY}" /workspace && \
    apk add libc6-compat tzdata
ARG TARGETOS
ARG TARGETARCH
COPY ${BINARY}-*-${TARGETOS}-${TARGETARCH} /opt/${BINARY}/
RUN chmod +x /opt/${BINARY}/${BINARY}-*-${TARGETOS}-${TARGETARCH} && \
    ln -s /opt/${BINARY}/${BINARY}-*-${TARGETOS}-${TARGETARCH} /opt/${BINARY}/${BINARY}
WORKDIR /workspace
ENTRYPOINT ["/opt/${BINARY}/${BINARY}"]
