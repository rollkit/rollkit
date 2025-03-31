FROM --platform=$BUILDPLATFORM golang:1.23-alpine AS build-env

# Set working directory for the build
WORKDIR /src

COPY . .

RUN go mod tidy -compat=1.19 && \
    go build /src/cmd/local-da/main.go

# Final image
FROM alpine:3.18.3

WORKDIR /root

# Copy over binaries from the build-env
COPY --from=build-env /src/main /usr/bin/local-da

EXPOSE 7980

ENTRYPOINT ["local-da"]
CMD ["-listen-all"]
