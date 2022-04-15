FROM --platform=$BUILDPLATFORM golang:1.17-alpine AS build-env

# Set working directory for the build
WORKDIR /src 

COPY . .

RUN cd cmd && \
    go mod tidy -compat=1.17 && \ 
    go build main.go

# Final image
FROM alpine

WORKDIR /root

# Copy over binaries from the build-env
COPY --from=build-env /src/cmd/main /usr/bin/mock-da

EXPOSE 7980

CMD ["mock-da"]
