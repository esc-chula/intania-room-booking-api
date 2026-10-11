FROM golang:1.26.8-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY docs ./docs
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

FROM alpine:3.23
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 app
WORKDIR /app
COPY --from=build /out/api ./api
USER app
ENV APP_ENV=production HTTP_ADDR=:8080
EXPOSE 8080
ENTRYPOINT ["/app/api"]
