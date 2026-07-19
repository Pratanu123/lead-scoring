FROM node:22-alpine AS web-build

WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.23-alpine AS go-build

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/api ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/worker ./cmd/worker

FROM alpine:3.21

WORKDIR /app
RUN apk add --no-cache ca-certificates
COPY --from=go-build /bin/api /app/api
COPY --from=go-build /bin/worker /app/worker
COPY --from=web-build /web/dist /app/web/dist

ENV STATIC_DIR=/app/web/dist
EXPOSE 8080

CMD ["/app/api"]
