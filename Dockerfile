## prep the base image.
#
FROM golang as base

RUN apt update && \
	apt-get install -y \
	build-essential \
	ca-certificates \
	curl

# enable faster module downloading.
ENV GOPROXY https://proxy.golang.org

## builder stage.
#
FROM base as builder

WORKDIR /rollkit

# cache dependencies.
COPY ./go.mod . 
COPY ./go.sum . 
RUN go mod download

COPY . .

RUN make install

## prep the final image.
#
FROM base

COPY --from=builder /go/bin/rollkit /usr/bin

WORKDIR /apps

ENTRYPOINT ["rollkit"]
