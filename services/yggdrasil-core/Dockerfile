FROM golang:1.25 AS build

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o yggdrasil-core .

FROM build AS run

WORKDIR /app

COPY --from=build /build/yggdrasil-core .

EXPOSE 9080
ENTRYPOINT ["/app/yggdrasil-core"]
