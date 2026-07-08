FROM alpine:3.22

RUN apk add --no-cache fio \
  && mkdir -p /bench

WORKDIR /bench

ENTRYPOINT ["fio"]