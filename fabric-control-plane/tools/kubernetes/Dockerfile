FROM hyperledger/fabric-tools:2.5.15 AS fabrictools
FROM sathonate/bevel-tools:dev-20261016 AS worker
USER root
# Required protected stdin transport; build from the pinned source first.
COPY bin/kubectl-hlf-scoped /usr/local/bin/kubectl-hlf
COPY bin/fabric-worker /usr/local/bin/fabric-worker
COPY --from=fabrictools /usr/local/bin/peer /usr/local/bin/peer
COPY --from=fabrictools /etc/hyperledger/fabric/core.yaml /etc/hyperledger/fabric/core.yaml
USER 10001:10001
ENV ANSIBLE_HOME=/tmp/ansible FABRIC_CFG_PATH=/etc/hyperledger/fabric
ENTRYPOINT ["/usr/local/bin/fabric-worker"]

FROM gcr.io/distroless/static-debian12:nonroot AS smoke
COPY bin/local-chaincode /local-chaincode
USER 65532:65532
ENTRYPOINT ["/local-chaincode"]
