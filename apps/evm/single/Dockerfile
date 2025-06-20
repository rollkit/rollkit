FROM golang:1.24-alpine AS build-env

WORKDIR /src

ADD core core

COPY go.mod go.sum ./
RUN go mod download

COPY . .

WORKDIR /src/apps/evm/single
RUN go mod tidy && CGO_ENABLED=0 GOOS=linux go build -o evm-single .

FROM alpine:3.18.3

RUN apk --no-cache add ca-certificates

WORKDIR /root

COPY --from=build-env /src/apps/evm/single/evm-single /usr/bin/evm-single
COPY apps/evm/single/entrypoint.sh /usr/bin/entrypoint.sh
RUN chmod +x /usr/bin/entrypoint.sh

ENTRYPOINT ["/usr/bin/entrypoint.sh"]
