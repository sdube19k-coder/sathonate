FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
COPY controller-candidate /controller-candidate
COPY smp-candidate-build.json /source-evidence.json
COPY charts /charts
COPY LICENSE /licenses/LICENSE
COPY CREDITS /licenses/CREDITS
USER 65532:65532
ENTRYPOINT ["/controller-candidate"]
