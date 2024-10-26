package worker

import (
	"encoding/json"
	"log"

	"github.com/mini-maxit/worker/models"
	"github.com/mini-maxit/worker/repositories"
	"github.com/mini-maxit/worker/solution"
	"github.com/mini-maxit/worker/utils"
	amqp "github.com/rabbitmq/amqp091-go"
	"gorm.io/gorm"
)

type QueueMessage struct {
	TaskID         int `json:"task_id"`
	UserID         int `json:"user_id"`
	UserSolutionID int `json:"user_solution_id"`
}

// Maximum of retries on the same message
const maxRetries = 2

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
	if err != nil {
		log.Fatalf("Failed to declare a queue: %s", err)
	}

	// Consume messages from the queue
	msgs, err := ch.Consume(
		q.Name, // queue
		"",     // consumer
		false,  // auto-ack to be able to handle errors and requeue
		false,  // exclusive
		false,  // no-local
		false,  // no-wait
		nil,    // args
	)
	if err != nil {
		// Placeholder for custom error logger
		log.Fatalf("Failed to register a consumer: %s", err)
	}

	var forever = make(chan struct{})

	go func() {
		for msg := range msgs {
			func(msg amqp.Delivery) {
				var queueMessage QueueMessage
				log.Printf("Received a message")

				// Unmarshal the message body
				err := json.Unmarshal(msg.Body, &queueMessage)
				if err != nil {
					handleError(nil, db, &msg, ch, uint(queueMessage.UserSolutionID), err)
				}

				// Start a transaction
				tx := db.Begin()
				if tx.Error != nil {
					handleError(nil, db, &msg, ch, uint(queueMessage.UserSolutionID), tx.Error)
				}

				defer func() {
					if r := recover(); r != nil {
						msg.Ack(false)
						tx.Rollback()
						log.Printf("Recovered from panic: %v", r)
					}
				}()

				// Process the message
				err = processMessage(queueMessage, tx)
				if err != nil {
					handleError(tx, db, &msg, ch, uint(queueMessage.UserSolutionID), err)
				} else {
					msg.Ack(false)
					tx.Commit()
				}

				log.Printf("Processed message")
			}(msg)
		}
	}()

	// Placeholder for custom error logger
	log.Printf(" [*] Waiting for messages. To exit press CTRL+C")
	<-forever
}

// Process the incoming message
func processMessage(queueMessage QueueMessage,tx *gorm.DB) error {

	// Get the configuration data needed to run the solution
	task, err := getDataForSolutionRunner(tx, queueMessage.TaskID, queueMessage.UserSolutionID)
	if err != nil {
		return err
	}


	// Mark user solution as processing
	err = repositories.MarkUserSolutionProcessing(tx, queueMessage.UserSolutionID)
	if err != nil {
		return err
	}


	// Create a new solution and run it
	solutionResult, err := runSolution(task)
	if err != nil {
		return err
	}

	// Remove the temporary directorie
	utils.RemoveDir(task.TempDir, true, true)

	// Create a new user solution result and test results
	err = CreateUserSolutionResultAndTestResults(tx, uint(queueMessage.UserSolutionID), solutionResult, queueMessage)
	if err != nil {
		return err
	}

	// Mark the solution as complete
	err = repositories.MarkUserSolutionComplete(tx, uint(queueMessage.UserSolutionID))
	if err != nil {
		return err
	}

	return nil
}

// Handle errors and requeue the message if needed
func handleError(tx *gorm.DB,  db *gorm.DB, msg *amqp.Delivery, ch *amqp.Channel, user_solution_id uint, err error) {

	// Rollback any active transaction if needed
	if tx != nil && tx.Error == nil {
		tx.Rollback()
	}

	newMsg := getNewMsg(msg)
	if newMsg.Body == nil {
		// Placeholder for custom error logger
		log.Printf("Dropping message after 3 retries")
		repositories.MarkUserSolutionFailed(db, user_solution_id, err)

		// Message was retried maxRetries times, ack it and remove it from the queue
		msg.Ack(false)
		return
	}

	// The original message was not processed successfully, acknowledge it and send an updated message to the queue
	msg.Ack(false)

	// Send the updated message to the queue
	err = ch.Publish(
		"",             // exchange
		"worker_queue", // routing key
		false,          // mandatory
		false,          // immediate
		amqp.Publishing{
			ContentType: "application/json",
			Body:        newMsg.Body,
			Headers:     newMsg.Headers,
		})
	if err != nil {
		// Placeholder for custom error logger
		log.Printf("Failed to send message to the queue: %s", err)
		repositories.MarkUserSolutionFailed(db, user_solution_id, err)
	}
}

// Get the a new messsage to be requeued
func getNewMsg(msg *amqp.Delivery) amqp.Delivery {
	oldHeaderCount := msg.Headers["x-retry-count"]

	if oldHeaderCount.(int64) >= maxRetries {
		return amqp.Delivery{}
	}
	newHeaderCount := oldHeaderCount.(int64) + 1

	newMsg := amqp.Delivery{Body: msg.Body, Headers: amqp.Table{"x-retry-count": newHeaderCount}}
	return newMsg
}

// Run the solution using the solution runner
func runSolution(task TaskForRunner) (solution.SolutionResult, error) {
	runner := solution.Runner{}
	langType, err := solution.StringToLanguageType(task.LanguageType)
	if err != nil {
		return solution.SolutionResult{}, err
	}

	langConfig := solution.LanguageConfig{
		Type:    langType,
		Version: task.LanguageVersion,
	}

	solution := solution.Solution{
		Language:         langConfig,
		BaseDir:          task.BaseDir,
		SolutionFileName: task.SolutionFileName,
		InputDir:         task.StdinDir,
		OutputDir:        task.ExpectedOutputsDir,
	}

	solutionResult := runner.RunSolution(&solution)

	return solutionResult, nil
}

// Create a new user solution result and test results
func CreateUserSolutionResultAndTestResults(tx *gorm.DB, user_solution_id uint, solutionResult solution.SolutionResult, queueMessage QueueMessage) error {

	dbSolutionResult := models.UserSolutionResult{
		UserSolutionID: user_solution_id,
		Code:           solutionResult.Code,
		Message:        solutionResult.Message,
	}

	user_solution_result_id, err := repositories.CreateUserSolutionResult(tx, dbSolutionResult)
	if err != nil {
		return err
	}

	for _, testResult := range solutionResult.TestResults {
		input_outpu_id, err := repositories.GetInputOutputId(tx, queueMessage.TaskID, testResult.Order)
		if err != nil {
			return err
		}
		dbTestResult := models.TestResult{
			UserSolutionResultID: user_solution_result_id,
			OutputFilePath:       testResult.ActualFile,
			Passed:               testResult.Passed,
			ErrorMessage:         testResult.ErrorMessage,
			InputOutputID:        uint(input_outpu_id),
		}
		err = repositories.CreateTestResults(tx, dbTestResult)
		if err != nil {
			return err
		}
	}
	return nil
}
