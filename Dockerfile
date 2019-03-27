FROM golang:1.10.4 as builder
COPY . /go/src/caksu
WORKDIR /go/src/caksu
RUN CGO_ENABLED=0 GOOS=linux go build -o caksu ./cmd/...

FROM alpine:latest
LABEL maintainer="vvianzhang@gmail.com"
WORKDIR /root/
COPY --from=builder /go/src/caksu/caksu .
CMD ["./caksu"]
