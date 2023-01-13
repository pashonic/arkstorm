FROM golang:1.19.4-alpine AS builder
RUN apk add --update make
WORKDIR /go/arkstorm
COPY . ./
RUN unset GOPATH
RUN make build

# BUGBUG: Make this more secure
FROM alpine:latest  
RUN apk --no-cache add ca-certificates ffmpeg tzdata aws-cli bash
WORKDIR /root/
COPY --from=builder /go/arkstorm/entrypoint.sh ./
COPY --from=builder /go/arkstorm/bin/arkstorm ./
COPY --from=builder /go/arkstorm/fonts/Yagora.ttf ./fonts/
RUN chmod +x entrypoint.sh
CMD ["./entrypoint.sh"]