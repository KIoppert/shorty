FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /shorty .

FROM alpine:3.22
RUN adduser -D -H -u 10001 app
USER 10001
COPY --from=build /shorty /usr/local/bin/shorty
EXPOSE 8080 9091
ENTRYPOINT ["shorty"]
CMD ["serve"]
