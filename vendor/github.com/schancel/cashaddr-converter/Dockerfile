FROM golang:alpine
RUN mkdir -p /go/src/github.com/schancel/cashaddr-converter/
WORKDIR /go/src/github.com/schancel/cashaddr-converter/
COPY . .
RUN go install github.com/schancel/cashaddr-converter/cmd/svc
FROM alpine
COPY --from=0 /go/bin/svc /svc
COPY --from=0 /go/src/github.com/schancel/cashaddr-converter/static /static
CMD ["/svc"]