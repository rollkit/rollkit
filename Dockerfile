## prep the base image.
#
FROM golang AS base

RUN apt update && \
	apt-get install -y \
	build-essential \
	ca-certificates \
	curl

# enable faster module downloading.
ENV GOPROXY=https://proxy.golang.org

## builder stage.
#
FROM base AS builder

WORKDIR /rollkit

# Copiar todo el código fuente primero
COPY . .

# Ahora descargar las dependencias
RUN go mod download

RUN make install

## prep the final image.
#
FROM base

COPY --from=builder /go/bin/rollkit /usr/bin

WORKDIR /apps

ENTRYPOINT ["rollkit"]
