# syntax=docker/dockerfile:1.2
FROM bitnami/git
COPY . /
ENV GIT_SSH_COMMAND "ssh -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no"
RUN --mount=type=ssh git clone git@github.com:RutledgePaulV/terraform-provider-openpgp.git