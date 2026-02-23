FROM golang:1.22-alpine AS build
WORKDIR /src

ARG TARGETOS
ARG TARGETARCH

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-arm64} go build -o /out/meituanone ./cmd/server

FROM alpine:3.20
WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata

COPY --from=build /out/meituanone /app/meituanone
COPY web /app/web

ENV PORT=3000
ENV WEB_DIR=/app/web
ENV DB_PATH=/app/data/shop.db
ENV STORAGE_PROFILE=low_write
ENV ACCESS_LOG=false
ENV GIN_MODE=release
ENV STORE_NAME=Demo\ Store
ENV ADMIN_USER=admin
ENV ADMIN_PASSWORD=admin123
ENV JWT_SECRET=replace-me-in-production
ENV TOKEN_TTL_HOURS=720
ENV AUTO_PRINT=true
ENV PRINTER_MODE=stdout
ENV PRINTER_DEVICE=/dev/usb/lp0
ENV PRINTER_TCP=
ENV CORS_ORIGIN=*

VOLUME ["/app/data"]
EXPOSE 3000

CMD ["/app/meituanone"]
