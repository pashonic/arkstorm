FROM golang:1.19.4-alpine AS builder
WORKDIR /go/arkstorm
COPY . ./
RUN unset GOPATH
RUN make build

# BUGBUG: Make this more secure
FROM alpine:latest  
RUN apk --no-cache add ca-certificates ffmpeg
WORKDIR /root/
COPY --from=builder /go/arkstorm/bin/arkstorm ./
CMD ["./arkstorm"]
