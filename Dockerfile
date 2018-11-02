FROM golang:1.10.4
LABEL maintainer="vvianzhang@gmail.com"

RUN apt-get update && \
    apt-get install -y dnsutils && \
    rm -rf /var/lib/apt/lists/*

ADD . $GOPATH/src/github.com/caksu

WORKDIR $GOPATH/src/github.com/caksu

RUN cp -r vendor/k8s.io $GOPATH/src && \
    cp -r vendor/github.com $GOPATH/src

RUN go build .

ENTRYPOINT ["./caksu"]
