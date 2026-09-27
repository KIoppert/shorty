FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /shorty .

FROM alpine:3.22
RUN adduser -D -H app
USER app
COPY --from=build /shorty /usr/local/bin/shorty
EXPOSE 8080
ENTRYPOINT ["shorty"]
CMD ["serve"]
