## prep the base image.
#
FROM golang:1.24 AS base

RUN apt-get update && \
	apt-get install -y --no-install-recommends \
	build-essential \
	ca-certificates \
	curl \
	&& rm -rf /var/lib/apt/lists/*

# enable faster module downloading.
ENV GOPROXY=https://proxy.golang.org

## builder stage.
#
FROM base AS builder

WORKDIR /rollkit

# Copy all source code first
COPY . .

# Now download dependencies and build
RUN go mod download && make install

## prep the final image.
#
FROM base

COPY --from=builder /go/bin/testapp /usr/bin

WORKDIR /apps

ENTRYPOINT ["testapp"]
