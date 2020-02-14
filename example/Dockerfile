FROM alpine:latest as foo
WORKDIR /app
COPY example.sh /bin
ENTRYPOINT ["/bin/example.sh"]
ENV EXAMPLE="Some data"

FROM foo
LABEL foo=bar
