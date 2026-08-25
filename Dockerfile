FROM golang:1.26-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /tracks .

FROM alpine:3.22

WORKDIR /app
RUN mkdir -p /app/data

COPY --from=build /tracks /usr/local/bin/tracks

EXPOSE 8080
VOLUME ["/app/data"]

ENTRYPOINT ["tracks"]
