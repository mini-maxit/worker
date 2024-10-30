FROM golang:1.23

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN chmod +x wait-for-it.sh
RUN chmod +x app_entry_script.sh

RUN CGO_ENABLED=0 GOOS=linux go build -o bin/worker-service ./main.go

ENTRYPOINT [ "./app_entry_script.sh" ]
