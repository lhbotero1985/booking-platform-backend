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

USER nonroot:nonroot
EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/api"]
