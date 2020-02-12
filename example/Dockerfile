FROM alpine:latest as foo
WORKDIR /app
COPY example.sh /bin
ENTRYPOINT ["/bin/example.sh"]

FROM foo
LABEL foo=bar
