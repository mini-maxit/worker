package worker

import (
	"encoding/json"
	"log"

	amqp "github.com/rabbitmq/amqp091-go"
	"gorm.io/gorm"
)

// Work starts the worker process
func work() {
	db := connectToDatabase()

	conn, ch := connectToRabbitMQ()
	defer conn.Close()
	defer ch.Close()

	// Declare a queue
	q, err := ch.QueueDeclare(
		"worker_queue", // name
		false,          // durable
		false,          // delete when unused
		false,          // exclusive
		false,          // no-wait
		nil,            // arguments
	)
	failOnError(err, "Failed to declare a queue")

	// Consume messages from the queue
	msgs, err := ch.Consume(
		q.Name, // queue
		"",     // consumer
		true,   // auto-ack
		false,  // exclusive
		false,  // no-local
		false,  // no-wait
		nil,    // args
	)
	failOnError(err, "Failed to register a consumer")

	var forever = make(chan struct{})

	go func() {
		for msg := range msgs {
			log.Printf("Processing message")
			processMessage(msg, db)
			log.Printf("Message processed")
		}
	}()

	log.Printf(" [*] Waiting for messages. To exit press CTRL+C")
	<-forever
}

func processMessage(msg amqp.Delivery, db *gorm.DB) {
	var solution Solution
	err := json.Unmarshal(msg.Body, &solution)
	failOnError(err, "Failed to unmarshal the message body")
	log.Printf("Received a message: %v", solution)

	task, err := getDataForSolutionRunner(db, solution.TaskId, solution.UserId, solution.UserSolutionId, solution.InputOutputId)
	failOnError(err, "Failed to get data for solution runner")
	log.Printf("Data for solution runner: %v", task)

	err = createSolution(db, &solution)
	failOnError(err, "Failed to mark solution as processing")
	log.Printf("Solution created: %v", solution)

	solutionResult, err := runSolution(task, solution.UserSolutionId)
	failOnError(err, "Failed to run the solution")
	log.Printf("Solution result: %v", solutionResult)

	err = storeSolutionResult(db, solutionResult)
	failOnError(err, "Failed to store the solution result")
	log.Printf("Solution result stored")

	err = markSolutionComplete(db, solution.Id)
	failOnError(err, "Failed to mark solution as complete")
	log.Printf("Solution marked as complete")
}