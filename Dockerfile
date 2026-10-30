FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download || true
COPY . .
RUN go mod tidy && CGO_ENABLED=0 go build -o /out/server .

FROM alpine:3.19
RUN adduser -D app
USER app
COPY --from=build /out/server /server
EXPOSE 5003
ENTRYPOINT ["/server"]
