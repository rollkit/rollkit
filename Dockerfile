## prep the base image.
#
FROM golang AS base

RUN apt-get update && \
	apt-get install -y --no-install-recommends \
	build-essential=12.9 \
	ca-certificates=20211016 \
	curl=7.74.0-1.3+deb11u7 \
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
RUN go mod download && \
    make install

## prep the final image.
#
FROM base

COPY --from=builder /go/bin/rollkit /usr/bin

WORKDIR /apps

ENTRYPOINT ["rollkit"]
