# syntax=docker/dockerfile:1.2
FROM busybox:latest
COPY . /
RUN --mount=type=secret,id=mysecret cat /run/secrets/mysecret > /secret.txt