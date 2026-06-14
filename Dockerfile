FROM golang:1.22-alpine AS build

WORKDIR /src
COPY go.mod ./
COPY . .
RUN go test ./... && CGO_ENABLED=0 go build -o /out/pay233-server ./cmd/pay233-server

FROM alpine:3.22

WORKDIR /app
COPY --from=build /out/pay233-server /pay233-server
COPY config.example.json /app/config.example.json
EXPOSE 5500
ENTRYPOINT ["/pay233-server"]
