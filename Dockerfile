FROM golang:1.23

RUN apt-get update && apt-get install -y \
    git \
    g++

WORKDIR /maxit/worker

COPY go.mod .
RUN go mod download && go mod verify

COPY . .
CMD ["go", "run", "cmd/main.go"]
