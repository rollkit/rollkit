FROM --platform=$BUILDPLATFORM golang:1.17-alpine AS build-env

# Set working directory for the build
WORKDIR /src 

COPY . .

RUN cd cmd && go mod download 

# Make the binary
RUN go build /src/cmd/main.go

# Final image
FROM alpine

WORKDIR /root

# Copy over binaries from the build-env
COPY --from=build-env /src/main /usr/bin/mock-da

EXPOSE 7980

CMD ["mock-da"]
