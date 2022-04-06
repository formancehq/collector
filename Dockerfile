FROM golang:1.18-alpine as builder
ADD . /src
WORKDIR /src
RUN go build main.go

FROM alpine:3.15
COPY --from=builder /src/main /bin/benthos
ENTRYPOINT ["/bin/benthos"]
CMD ["--help"]