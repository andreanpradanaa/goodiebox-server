# Build goodiebox-server untuk Railway.
# Binary statis kecil, migrasi ter-embed — tidak butuh file ekstra saat runtime.
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/server ./cmd/server

FROM alpine:3.20
RUN adduser -D -u 10001 appuser
COPY --from=build /out/server /server
USER appuser
EXPOSE 8080
ENTRYPOINT ["/server"]
