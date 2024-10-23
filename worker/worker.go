package worker

import (
	"encoding/json"
	"log"

	amqp "github.com/rabbitmq/amqp091-go"
	"gorm.io/gorm"
	"github.com/mini-maxit/worker/models"
	"github.com/mini-maxit/worker/repositories"
)

// Maximum of retries on the same message
const maxRetries = 3

// Work starts the worker process
func Work(db *gorm.DB, conn *amqp.Connection, ch *amqp.Channel) {

	// Declare a queue
	q, err := ch.QueueDeclare(
		"worker_queue", // name
		false,          // durable
		false,          // delete when unused
		false,          // exclusive
		false,          // no-wait
		nil,            // arguments
	)
	if(err != nil) {
		log.Fatalf("Failed to declare a queue: %s", err)
	}

	// Consume messages from the queue
	msgs, err := ch.Consume(
		q.Name, // queue
		"",     // consumer
		false,   // auto-ack to be able to handle errors and requeue
		false,  // exclusive
		false,  // no-local
		false,  // no-wait
		nil,    // args
	)
	if(err != nil) {
		// Placeholder for custom error logger
		log.Fatalf("Failed to register a consumer: %s", err)
	}

	var forever = make(chan struct{})

	go func() {
		for msg := range msgs {
			processMessage(msg, db)
		}
	}()

	// Placeholder for custom error logger
	log.Printf(" [*] Waiting for messages. To exit press CTRL+C")
	<-forever
}

func processMessage(msg amqp.Delivery, db *gorm.DB) {
	var solution models.Solution

	// Unmarshal the message
	err := json.Unmarshal(msg.Body, &solution)
	if err != nil {
		handleError(msg, err, "Failed to unmarshal message")
		return
	}

	// Start a new transaction
	tx := db.Begin()
	if tx.Error != nil {
		handleError(msg, tx.Error, "Failed to start transaction")
		return
	}

	// Defer rollback in case of panic
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			log.Printf("Recovered from panic: %v", r)
		}
	}()

	// Get the task and data for the solution runner
	task, err := getDataForSolutionRunner(tx, solution.TaskId, solution.UserSolutionId, solution.InputOutputId)
	if err != nil {
		handleError(msg, err, "Failed to get data for solution runner")
		tx.Rollback()
		return
	}

	// Create a new solution in the database marked as processing
	err = repositories.CreateSolution(tx, &solution)
	if err != nil {
		tx.Rollback()
		handleError(msg, err, "Failed to create solution")
		return
	}

	// Run the solution, forn now it's just a placeholder
	solutionResult, err := runSolution(task, solution.UserSolutionId)
	if err != nil {
		tx.Rollback()
		handleError(msg, err, "Failed to run solution")
		return
	}

	// Store the solution result
	err = repositories.StoreSolutionResult(tx, solutionResult)
	if err != nil {
		tx.Rollback()
		handleError(msg, err, "Failed to store solution result")
		return
	}

	// Mark the solution as complete
	err = repositories.MarkSolutionComplete(tx, solution.Id)
	if err != nil {
		tx.Rollback()
		handleError(msg, err, "Failed to mark solution as complete")
		return
	}

    // Commit the transaction
    if err := tx.Commit().Error; err != nil {
        handleError(msg, err, "Failed to commit transaction")
        return
    }

	// Message was processed successfully, acknowledge it and remove it from the queue
    msg.Ack(false)
}


func handleError(msg amqp.Delivery,err error, errMsg string) {
	// Placeholder for custom error logger
    log.Printf("%s: %v", errMsg, err)

    retryCount := getRetryCount(msg)
    if retryCount >= maxRetries {
		// Placeholder for custom error logger
        log.Printf("Dropping message after %d retries", retryCount)
		// Message was retried maxRetries times, ack it and remove it from the queue
        msg.Ack(false)
        return
    }

	// Message was not processed successfully, nack it and requeue it
    msg.Nack(false, true)
}

func getRetryCount(msg amqp.Delivery) int {
    retryCount := 0
    if count, ok := msg.Headers["x-retry-count"]; ok {
        retryCount = count.(int)
    }
    retryCount++
    msg.Headers["x-retry-count"] = retryCount
    return retryCount
}

func runSolution(task models.Task, userSolutionId int) (models.SolutionResult, error) {
	// Placeholder for running the solution
	return models.SolutionResult{}, nil
}
