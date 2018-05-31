FROM golang:latest as buildstage
WORKDIR /go/src/cluster-watcher
ADD . .
# RUN go test ./...
RUN curl https://raw.githubusercontent.com/golang/dep/master/install.sh | sh
RUN dep ensure
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o cluster-watcher .

FROM alpine:latest  
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=buildstage /go/src/cluster-watcher/cluster-watcher /bin/cluster-watcher
CMD ["/bin/cluster-watcher"]  