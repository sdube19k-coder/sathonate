# Build context contains the Linux static server and a production Vite dist directory.
FROM scratch
COPY fabric-console-server /fabric-console-server
COPY dist/ /console/
USER 10001:10001
ENTRYPOINT ["/fabric-console-server"]
