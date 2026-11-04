FROM python:3.12-slim-bookworm@sha256:34386ef0cb081344d7ec1c103ba398e6e9f64e9ab3a1509accc92a4e24a07258

ARG HELM_VERSION=v3.19.0
ARG KUBECTL_VERSION=v1.34.1
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl git openssh-client jq unzip && rm -rf /var/lib/apt/lists/*
RUN pip install --no-cache-dir ansible==12.0.0 ansible-core==2.19.2 kubernetes==33.1.0 hvac==2.3.0 jmespath==1.0.1 jsonschema==4.25.1
RUN curl -fsSLo /tmp/helm.tgz https://get.helm.sh/helm-${HELM_VERSION}-linux-amd64.tar.gz \
 && curl -fsSLo /tmp/helm.sha https://get.helm.sh/helm-${HELM_VERSION}-linux-amd64.tar.gz.sha256sum \
 && cd /tmp && echo "$(cut -d ' ' -f 1 helm.sha)  helm.tgz" | sha256sum -c - \
 && tar -xzf helm.tgz && install linux-amd64/helm /usr/local/bin/helm
RUN curl -fsSLo /tmp/kubectl https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/amd64/kubectl \
 && curl -fsSLo /tmp/kubectl.sha https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/amd64/kubectl.sha256 \
 && cd /tmp && echo "$(cat kubectl.sha)  kubectl" | sha256sum -c - \
 && install kubectl /usr/local/bin/kubectl
RUN useradd --create-home --uid 10001 sathonate
COPY inventory.ini /opt/sathonate/inventory.ini
USER 10001:10001
ENV HOME=/home/sathonate ANSIBLE_INVENTORY=/opt/sathonate/inventory.ini ANSIBLE_LOCAL_TEMP=/tmp/ansible-local
WORKDIR /workspace
CMD ["ansible-playbook", "--version"]
