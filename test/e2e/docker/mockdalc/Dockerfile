FROM golang:1.16

RUN apt-get -qq update -y && apt-get -qq upgrade -y >/dev/null
RUN apt-get -qq install -y libleveldb-dev librocksdb-dev >/dev/null

WORKDIR /src/optimint

RUN mkdir build

# Fetch dependencies separately (for layer caching)
COPY go.mod go.sum ./
RUN go mod download

COPY . .

WORKDIR /src/optimint/test/e2e
RUN make mock-dalc && cp ./build/dalc /usr/bin/dalc


EXPOSE 7980
CMD ["dalc"]
STOPSIGNAL SIGTERM