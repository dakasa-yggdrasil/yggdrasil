FROM golang:1.25-bookworm

WORKDIR /app

COPY go.mod ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /bin/__SURFACE_NAME__ .

CMD ["/bin/__SURFACE_NAME__"]
