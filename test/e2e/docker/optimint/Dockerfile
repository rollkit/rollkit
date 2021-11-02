FROM golang:1.16

RUN apt-get -qq update -y && apt-get -qq upgrade -y >/dev/null
RUN apt-get -qq install -y libleveldb-dev librocksdb-dev >/dev/null

WORKDIR /src/optimint

RUN mkdir build

# Fetch dependencies separately (for layer caching)
COPY go.mod go.sum ./
RUN go mod download

# copy optimint
COPY . .

ENTRYPOINT ["/src/optimint/test/e2e/docker/optimint/docker-entrypoint.sh"]

WORKDIR /src/optimint/test/e2e

RUN make rollup && cp build/rollupd /usr/bin/rollupd

WORKDIR /optimint
VOLUME /optimint

EXPOSE 26656 26657 9090

CMD ["start-optimint"]
STOPSIGNAL SIGTERM