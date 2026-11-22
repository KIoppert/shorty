FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=secret,id=build_ca \
    if [ -f /run/secrets/build_ca ]; then export SSL_CERT_FILE=/run/secrets/build_ca; fi; \
    go mod download
COPY . .
ARG APP_VERSION=dev
RUN CGO_ENABLED=0 go build -mod=readonly -trimpath -ldflags="-s -w -X main.version=${APP_VERSION}" -o /shorty .

FROM alpine:3.22
RUN adduser -D -H -u 10001 app
USER 10001
COPY --from=build /shorty /usr/local/bin/shorty
EXPOSE 8080 9091
ENTRYPOINT ["shorty"]
CMD ["serve"]
