FROM alpine:latest

RUN apk --no-cache add ca-certificates

RUN adduser -D -u 1001 -g 1001 contentfulcommander

COPY contentfulcommander /usr/bin/

USER contentfulcommander
WORKDIR /home/contentfulcommander

ENTRYPOINT ["contentfulcommander"]
