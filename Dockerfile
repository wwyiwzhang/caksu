FROM golang:latest
LABEL maintainer="vvianzhang@gmail.com"

RUN apt-get update -y && \
    apt-get install -y dnsutils && \
    rm -rf /var/lib/apt/lists/*

ADD . $GOPATH/src/github.com/job-cleaner

WORKDIR $GOPATH/src/github.com/job-cleaner

RUN go get github.com/golang/glog && \
    go get k8s.io/api && \
    go get k8s.io/apimachinery \
    go get k8s.io/client-go

RUN go build .

ENTRYPOINT ["./job-cleaner"]
