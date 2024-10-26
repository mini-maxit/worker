FROM golang:1.23-bullseye

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

COPY wait-for-it.sh /app/wait-for-it.sh
RUN chmod +x /app/wait-for-it.sh


RUN CGO_ENABLED=0 GOOS=linux go build -o /worker-service ./main.go

CMD ["./wait-for-it.sh", "postgres:5432", "--", "./wait-for-it.sh", "rabbitmq:5672", "--", "/worker-service"]
