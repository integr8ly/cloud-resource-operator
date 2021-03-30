FROM registry.access.redhat.com/ubi7/ubi-minimal:latest

ENV OPERATOR=/usr/local/bin/cloud-resource-operator \
    USER_UID=1001 \
    USER_NAME=cloud-resource-operator

# install operator binary
COPY build/_output/bin/cloud-resource-operator ${OPERATOR}

COPY build/bin /usr/local/bin
RUN  /usr/local/bin/user_setup

ENTRYPOINT ["/usr/local/bin/entrypoint"]

USER ${USER_UID}