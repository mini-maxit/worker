package worker

import (
	"encoding/json"
	"log"

	amqp "github.com/rabbitmq/amqp091-go"
	"gorm.io/gorm"
	"github.com/mini-maxit/worker/utils"
)

// Work starts the worker process
func Work() {
	
	
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
	utils.FailOnError(err, "Failed to declare a queue")

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
	utils.FailOnError(err, "Failed to register a consumer")

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
	utils.FailOnError(err, "Failed to unmarshal the message body")
	log.Printf("Received a message")

	task, err := getDataForSolutionRunner(db, solution.TaskId, solution.UserId, solution.UserSolutionId, solution.InputOutputId)
	utils.FailOnError(err, "Failed to get data for solution runner")
	log.Printf("Gathered data for solution runner")

	err = createSolution(db, &solution)
	utils.FailOnError(err, "Failed to mark solution as processing")
	log.Printf("Solution entry created")

	solutionResult, err := runSolution(task, solution.UserSolutionId)
	utils.FailOnError(err, "Failed to run the solution")
	log.Printf("Solution ran")

	err = storeSolutionResult(db, solutionResult)
	utils.FailOnError(err, "Failed to store the solution result")
	log.Printf("Solution result stored")

	err = markSolutionComplete(db, solution.Id)
	utils.FailOnError(err, "Failed to mark solution as complete")
	log.Printf("Solution marked as complete")
}