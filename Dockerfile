FROM golang:1.23

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

COPY wait-for-it.sh /app/wait-for-it.sh
RUN chmod +x /app/wait-for-it.sh 

COPY ./app_entery_script.sh /app/app_entery_script.sh
RUN chmod +x /app/app_entery_script.sh


RUN CGO_ENABLED=0 GOOS=linux go build -o /worker-service ./main.go

ENTRYPOINT [ "/app_entery_script.sh" ]

