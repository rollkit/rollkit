FROM golang:1.27.0-alpine AS build-env

WORKDIR /src

COPY core core

COPY go.mod go.sum ./
RUN go mod download

COPY . .

WORKDIR /src/apps/evm
RUN go mod tidy && CGO_ENABLED=0 GOOS=linux go build -o evm .

FROM alpine:3.22.2

#hadolint ignore=DL3018
RUN apk --no-cache add ca-certificates curl

WORKDIR /root

COPY --from=build-env /src/apps/evm/evm /usr/bin/evm
COPY apps/evm/entrypoint.sh /usr/bin/entrypoint.sh
RUN chmod +x /usr/bin/entrypoint.sh

ENTRYPOINT ["/usr/bin/entrypoint.sh"]
