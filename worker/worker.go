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
	TaskID 		    int `json:"task_id"`
	UserID 			int `json:"user_id"`
	UserSolutionID 	int	`json:"user_solution_id"`
}

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
	var queueMessage QueueMessage

	// Unmarshal the message
	err := json.Unmarshal(msg.Body, &queueMessage)
	if err != nil {
		handleError(nil, msg, queueMessage ,err, "Failed to unmarshal message")
		return
	}

	// Start a new transaction
	tx := db.Begin()
	if tx.Error != nil {
		handleError(tx, msg, queueMessage, tx.Error, "Failed to start transaction")
		return
	}

	// Defer rollback in case of panic
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			log.Printf("Recovered from panic: %v", r)
		}
	}()

	// Get the configuration data needed to run the solution
	task, err := getDataForSolutionRunner(tx, queueMessage.TaskID, queueMessage.UserSolutionID)
	if err != nil {
		handleError(tx, msg, queueMessage,err, "Failed to get data for solution runner")
		tx.Rollback()
		return
	}

	// Create a new user solution marked as processing
	user_solution_id, err := createUserSolution(tx, queueMessage, task)
	if (err != nil) {
		handleError(tx, msg, queueMessage ,err, "Failed to create solution")
		tx.Rollback()
		return
	}

	// Create a new solution and run it
	solutionResult, err := runSolution(task)
	if (err != nil) {
		handleError(tx, msg, queueMessage ,err, "Failed to run solution")
		tx.Rollback()
		return
	}

	// Remove the temporary directorie
	utils.RemoveDir(task.BaseDir, true, true)

	// Create a new user solution result and test results
	err = CreateUserSolutionResultAndTestResults(tx, msg, user_solution_id, solutionResult, queueMessage)
	if err != nil {
		handleError(tx, msg, queueMessage ,err, "Failed to create solution result and test results")
		tx.Rollback()
		return
	}

	// Mark the solution as complete
	err = repositories.MarkUserSolutionComplete(tx, uint(queueMessage.UserSolutionID))
	if err != nil {
		tx.Rollback()
		handleError(tx, msg, queueMessage , err, "Failed to mark solution as complete")
		return
	}

    // Commit the transaction
    if err := tx.Commit().Error; err != nil {
        handleError(tx, msg, queueMessage ,err, "Failed to commit transaction")
        return
    }

	// Message was processed successfully, acknowledge it and remove it from the queue
    msg.Ack(false)
}


// Helper functions to clean up the processMessage function
func handleError(tx *gorm.DB,msg amqp.Delivery,queueMessage QueueMessage,err error, errMsg string) {
	// Placeholder for custom error logger
    log.Printf("%s: %v", errMsg, err)

    retryCount := getRetryCount(msg)
    if retryCount >= maxRetries {
		// Placeholder for custom error logger
        log.Printf("Dropping message after %d retries", retryCount)
		repositories.MarkUserSolutionFailed(tx, uint(queueMessage.UserSolutionID), err)
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

func createUserSolution(tx *gorm.DB, queueMessage QueueMessage, task TaskForRunner) (uint, error){
	userSolution  := models.UserSolution{
		TaskID: uint(queueMessage.TaskID),
		SolutionFileName: task.SolutionFileName,
		LanguageType: task.LanguageType,
		LanguageVersion: task.LanguageVersion,
		Status: "processing",
	}

	user_solution_id, err := repositories.CreateUserSolution(tx, userSolution)
	if err != nil {
		return 0, err
	}

	return user_solution_id, nil
}

func runSolution(task TaskForRunner) (solution.SolutionResult, error) {
	runner := solution.Runner{}
	langType, err := solution.StringToLanguageType(task.LanguageType)
	if err != nil {
		return solution.SolutionResult{}, err
	}

	langConfig := solution.LanguageConfig{
		Type: langType,
		Version: task.LanguageVersion,
	}

	solution := solution.Solution{
		Language: langConfig,
		BaseDir: task.BaseDir,
		SolutionFileName: task.SolutionFileName,
		InputDir: task.StdinDir,
		OutputDir: task.ExpectedOutputsDir,
	}

	solutionResult := runner.RunSolution(&solution)

	return solutionResult, nil
}

func CreateUserSolutionResultAndTestResults(tx *gorm.DB, msg amqp.Delivery, user_solution_id uint, solutionResult solution.SolutionResult, queueMessage QueueMessage) error {

	dbSolutionResult := models.UserSolutionResult{
		UserSolutionID: user_solution_id,
		Code: solutionResult.Code,
		Message: solutionResult.Message,
	}

	user_solution_result_id, err := repositories.CreateUserSolutionResult(tx, dbSolutionResult)
	if err != nil {
		return err
	}

	for _, testResult := range solutionResult.TestResults {
		input_outpu_id, err := repositories.GetInputOutputId(tx, queueMessage.TaskID, testResult.Order)
		if(err != nil) {
			return err
		}
		dbTestResult := models.TestResult{
			UserSolutionResultID: user_solution_result_id,
			OutputFilePath: testResult.ActualFile,
			Passed: testResult.Passed,
			ErrorMessage: testResult.ErrorMessage,
			InputOutputID: uint(input_outpu_id),
		}
		err = repositories.CreateTestResults(tx, dbTestResult)
		if err != nil {
			return err
		}
	}

	return nil
}
