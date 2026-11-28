# syntax=docker/dockerfile:1

# ---- build ----------------------------------------------------------------
ARG GO_VERSION=1.27.4
FROM golang:${GO_VERSION}-alpine AS build

WORKDIR /src

# Dependencies first for better layer caching.
COPY go.mod go.sum* ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
COPY db ./db

ARG VERSION=dev
ARG COMMIT=unknown
ARG MODULE=github.com/lhbotero1985/booking-platform-backend

RUN CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X ${MODULE}/internal/platform/buildinfo.version=${VERSION} -X ${MODULE}/internal/platform/buildinfo.commit=${COMMIT}" \
      -o /out/ ./cmd/...
# The media volume's mount point, owned by nonroot: a new named volume copies it (and its owner).
RUN mkdir -p /out/data/media

# ---- deployment stack (data only) ------------------------------------------
FROM build AS stack
COPY deploy/compose/docker-compose.qa.yml /deploy/qa/docker-compose.yml
COPY deploy/compose/qa /deploy/qa/qa
RUN find /deploy -type d -exec chmod 0755 {} +  && find /deploy -type f -exec chmod 0644 {} +  && chmod 0755 /deploy/qa/qa/initdb/*.sh

# ---- runtime --------------------------------------------------------------
# distroless/static: no shell, no package manager, runs as uid 65532 (nonroot).
FROM gcr.io/distroless/static-debian12:nonroot

ARG VERSION=dev
ARG COMMIT=unknown
LABEL org.opencontainers.image.title="booking-platform-backend" \
      org.opencontainers.image.source="https://github.com/lhbotero1985/booking-platform-backend" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}"

COPY --from=build /out/api /usr/local/bin/api
COPY --from=build /out/worker /usr/local/bin/worker
COPY --from=build --chown=65532:65532 /out/data /data

# Deployment stack shipped inside the artifact (T3.5): the server's deploy script extracts it,
# so CI only sends an image digest, never files. Data only; nothing here is executed.
# Modes are set explicitly in the build stage: some BuildKit versions apply COPY --chmod to the
# implicitly created parent directories too (0644 directories cannot be traversed).
COPY --from=stack /deploy /deploy

USER nonroot:nonroot
EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/api"]
