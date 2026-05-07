FROM golang:1.23-alpine AS build

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/api ./cmd/api

FROM alpine:3.21

WORKDIR /app

COPY --from=build /bin/api /app/api

EXPOSE 8080

CMD ["/app/api"]
