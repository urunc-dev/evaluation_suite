#!/bin/sh
exec /docker-entrypoint.sh nginx \
  -g 'daemon off; error_log stderr notice;'