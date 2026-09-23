FROM golang:1.27.1-alpine3.24 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
ARG SERVICE=control-plane
RUN case "$SERVICE" in control-plane|worker|connector|migrate|audit-verify) ;; *) exit 1 ;; esac && \
    CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/app ./cmd/$SERVICE

FROM alpine:3.24
RUN addgroup -S app && adduser -S -G app app
COPY --from=build /out/app /app
COPY db /db
USER app
ENTRYPOINT ["/app"]
