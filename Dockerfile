FROM golang:1.22-alpine AS build
WORKDIR /src
COPY . .
RUN go mod tidy
RUN CGO_ENABLED=0 go build -o /out/app ./cmd/server

FROM alpine:3.20
COPY --from=build /out/app /app
ENTRYPOINT ["/app"]
