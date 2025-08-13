FROM gcr.io/distroless/base-debian12

# Build arguments
ARG BUILD_VERSION
ARG BUILD_DATE
ARG VCS_REF
ARG VCS_URL

# Labels
LABEL org.opencontainers.image.title="FND" \
      org.opencontainers.image.description="Frigate Notification Daemon" \
      org.opencontainers.image.vendor="Biberino" \
      org.opencontainers.image.source="${VCS_URL}" \
      org.opencontainers.image.revision="${VCS_REF}" \
      org.opencontainers.image.created="${BUILD_DATE}" \
      org.opencontainers.image.version="${BUILD_VERSION}"

WORKDIR /
ADD ./bin/fnd /fnd
EXPOSE 7777
CMD ["/fnd"]
