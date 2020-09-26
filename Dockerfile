FROM golang:alpine AS build

WORKDIR /workspace
RUN apk add --update --no-cache ca-certificates
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /bin/iglog-server ./cmd/iglog-server


FROM scratch

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /bin/iglog-server /bin/

ENTRYPOINT [ "/bin/iglog-server" ]
