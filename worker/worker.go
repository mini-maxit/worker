package worker

import (
	"encoding/json"
	"log"

	"github.com/mini-maxit/worker/solution"
	"github.com/mini-maxit/worker/utils"
	amqp "github.com/rabbitmq/amqp091-go"
)

type QueueMessage struct {
	MessageID 				string    `json:"message_id"`
	BackendResponseQueue    string    `json:"backend_response_queue"`
	TaskID         			int 	  `json:"task_id"`
	UserID         			int 	  `json:"user_id"`
	UserSolutionID 			int       `json:"user_solution_id"`
	LanguageType  			string 	  `json:"language_type"`
	LanguageVersion 		string    `json:"language_version"`
	SolutionFileName 		string    `json:"solution_file_name"`
	TimeLimits	  			[]float64 `json:"time_limits"`
	MemoryLimits	  		[]float64 `json:"memory_limits"`
	OutputDirName 			string    `json:"output_dir_name"`
	InputDirName 			string    `json:"input_dir_name"`
}

type ResponseMessage struct {
	MessageID	 	string 					`json:"message_id"`
	TaskID   		int 					`json:"task_id"`
	UserID  		int 					`json:"user_id"`
	UserSolutionID  int 					`json:"user_solution_id"`
	Result 			solution.SolutionResult `json:"result"`
}


// Maximum of retries on the same message
const maxRetries = 2

// Work starts the worker process
func Work(ch *amqp.Channel) {

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
					log.Printf("Failed to unmarshal the message: %s", err)
					handleError(QueueMessage{}, &msg, ch, err)
				}

				defer func() {
					if r := recover(); r != nil {
						msg.Ack(false)
						log.Printf("Recovered from panic: %v", r)
					}
				}()

				// Process the message
				err = processMessage(queueMessage, ch)
				if err != nil {
					handleError(queueMessage, &msg, ch, err)
				} else {
					msg.Ack(false)
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
func processMessage(queueMessage QueueMessage, ch *amqp.Channel) error {

	// Get the configuration data needed to run the solution
	task, err := getDataForSolutionRunner(queueMessage.TaskID, queueMessage.UserID, queueMessage.UserSolutionID)
	if err != nil {
		return err
	}

	task.LanguageType = queueMessage.LanguageType
	task.LanguageVersion = queueMessage.LanguageVersion
	task.SolutionFileName = queueMessage.SolutionFileName
	task.TimeLimits = queueMessage.TimeLimits
	task.MemoryLimits = queueMessage.MemoryLimits
	task.InputDirName = queueMessage.InputDirName
	task.OutputDirName = queueMessage.OutputDirName

	// Create a new solution and run it
	solutionResult, err := runSolution(task)
	if err != nil {
		return err
	}

	log.Printf("Solution result: %v", solutionResult)

	// Remove the temporary directorie
	utils.RemoveIO(task.TempDir, true, true)

	// Send the response message to the backend
	err = sendResponseMessage(queueMessage ,solutionResult, ch)
	if err != nil {
		return err
	}

	return nil
}

// Send response message to backend with solution result
func sendResponseMessage(queueMessage QueueMessage ,solutionResult solution.SolutionResult, ch *amqp.Channel) error {

	// Create a response message
	responseMessage := ResponseMessage{
		MessageID: queueMessage.MessageID,
		TaskID:   queueMessage.TaskID,
		UserID:  queueMessage.UserID,
		UserSolutionID: queueMessage.UserSolutionID,
		Result: solutionResult,
	}

	// Marshal the solution result
	solutionResultBytes, err := json.Marshal(responseMessage)
	if err != nil {
		return err
	}

	err = ch.Publish(
		"",               // exchange
		queueMessage.BackendResponseQueue, // routing key (queue name)
		false,            // mandatory
		false,            // immediate
		amqp.Publishing{
			ContentType: "application/json",
			Body:        solutionResultBytes,
		})
	if err != nil {
		log.Printf("Failed to send response message to the backend: %s", err)
		return err
	}

	return nil
}

// Handle errors and requeue the message if needed
func handleError(queueMessage QueueMessage, msg *amqp.Delivery, ch *amqp.Channel, err error) {

	// Placeholder for custom error logger
	log.Printf("Error processing message: %s", err)


	newMsg := getNewMsg(msg)
	if newMsg.Body == nil {
		// Placeholder for custom error logger
		log.Printf("Dropping message after 3 retries")

		failedSolutionResult := solution.SolutionResult{
			Success:    false,
			StatusCode: solution.InternalError,
			Code:       "500",
			Message:    "Failed to process the message after 3 retries: " + err.Error(),
			TestResults: nil,
		}

		// Send the response message to the backend
		err = sendResponseMessage(queueMessage, failedSolutionResult, ch)
		if err != nil {
			// Placeholder for custom error logger
			log.Printf("Failed to send response message to the backend: %s", err)
		}
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
		InputDir:         task.InputDirName,
		OutputDir:        task.OutputDirName,
	}

	solutionResult := runner.RunSolution(&solution)

	return solutionResult, nil
}
