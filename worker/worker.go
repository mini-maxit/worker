package worker

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/mini-maxit/worker/executor"
	"github.com/mini-maxit/worker/logger"
	"github.com/mini-maxit/worker/solution"
	"github.com/mini-maxit/worker/utils"
	amqp "github.com/rabbitmq/amqp091-go"
	log "github.com/sirupsen/logrus"
)

type QueueMessage struct {
	MessageID 				string    `json:"message_id"`
	TaskID         			int64 	  `json:"task_id"`
	UserID         			int64 	  `json:"user_id"`
	SubmissionNumber 		int64     `json:"submission_number"`
	LanguageType  			string 	  `json:"language_type"`
	LanguageVersion 		string    `json:"language_version"`
	TimeLimits	  			[]float64 `json:"time_limits"`
	MemoryLimits	  		[]float64 `json:"memory_limits"`
}

type ResponseMessage struct {
	MessageID	 	string 					`json:"message_id"`
	Result 			solution.SolutionResult `json:"result"`
}

// Base name for the solution file
const solutionFileBaseName = "solution"

// Input directory name
const inputDirName = "inputs"

// Output directory name
const outputDirName = "outputs"

// Maximum of retries on the same message
const maxRetries = 2

// Error message for failed to store the solution result
var errorFailedToStore = errors.New("failed to store the solution result")

// Work starts the worker process
func Work(conn *amqp.Connection) {
	logger := logger.NewNamedLogger("worker")

	ch := NewRabbitMQChannel(conn)
	defer func() {
		logger.Info("Worker exiting. Closing the channel")
		ch.Close()
	}()


	logger.Info("Worker started")

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
		logger.Fatalf("Failed to declare a queue: %s", err)
	}

	logger.Info("Queue declared")

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
		logger.Fatalf("Failed to register a consumer: %s", err)
	}

	var forever = make(chan struct{})

	go func() {
		for msg := range msgs {
			func(msg amqp.Delivery) {
				var queueMessage QueueMessage

				logger.Info("Received a message")

				// Unmarshal the message body
				err := json.Unmarshal(msg.Body, &queueMessage)
				if err != nil {
					handleError(QueueMessage{}, &msg, ch, err, logger)
				}

				logger.Infof("Processing message [MsgID: %s]", queueMessage.MessageID)

				defer func() {
					if r := recover(); r != nil {
						msg.Ack(false)
						logger.Errorf("Recovered from panic: %v", r)
					}
				}()

				// Process the message
				err = processMessage(queueMessage, &msg, ch, logger)
				if err != nil {
					handleError(queueMessage, &msg, ch, err, logger)
				} else {
					msg.Ack(false)
				}

				logger.Infof("Processed message [MsgID: %s]", queueMessage.MessageID)
			}(msg)
		}
	}()

	logger.Info("[*] Waiting for messages")
	<-forever
}

// Process the incoming message
func processMessage(queueMessage QueueMessage, msg *amqp.Delivery ,ch *amqp.Channel, logger *log.Entry) error {

	logger.Infof("Getting data for solution runner [MsgID: %s]", queueMessage.MessageID)
	// Get the configuration data needed to run the solution
	task, err := getDataForSolutionRunner(queueMessage.TaskID, queueMessage.UserID, queueMessage.SubmissionNumber)
	if err != nil {
		return err
	}

	// Remove the temp directory after the task is done
	defer utils.RemoveIO(task.TempDir, true, true)

	// Get the language type
	task.LanguageType, err = solution.StringToLanguageType(queueMessage.LanguageType)
	if err != nil {
		return err
	}

	// Get the solution file name with the correct extension
	task.SolutionFileName, err = solution.GetSolutionFileNameWithExtension(solutionFileBaseName, task.LanguageType)
	// Get the solution file name with the correct extension
	if err != nil {
		return err
	}

	task.LanguageVersion = queueMessage.LanguageVersion
	task.TimeLimits = queueMessage.TimeLimits
	task.MemoryLimits = queueMessage.MemoryLimits
	task.InputDirName = inputDirName
	task.OutputDirName = outputDirName

	logger.Infof("Data for solution runner retrieved [MsgID: %s]", queueMessage.MessageID)

	logger.Infof("Running solution [MsgID: %s]", queueMessage.MessageID)

	// Create a new solution and run it
	solutionResult, err := runSolution(task, queueMessage.MessageID)
	if err != nil {
		return err
	}

	logger.Infof("Solution ran successfully [MsgID: %s]", queueMessage.MessageID)

	logger.Infof("Storing solution result [MsgID: %s]", queueMessage.MessageID)

	// Store the solution result
	err = storeSolutionResult(solutionResult ,task, queueMessage)
	if err != nil {
		return err
	}

	logger.Infof("Solution result stored [MsgID: %s]", queueMessage.MessageID)

	logger.Infof("Sending response message [MsgID: %s]", queueMessage.MessageID)

	// Send the response message to the backend
	err = sendResponseMessage(queueMessage ,solutionResult, msg ,ch)
	if err != nil {
		return err
	}

	logger.Infof("Response message sent [MsgID: %s]", queueMessage.MessageID)

	return nil
}

// Send response message to backend with solution result
func sendResponseMessage(queueMessage QueueMessage ,solutionResult solution.SolutionResult, msg *amqp.Delivery,ch *amqp.Channel) error {

	// Create a response message
	responseMessage := ResponseMessage{
		MessageID: queueMessage.MessageID,
		Result: solutionResult,
	}

	// Marshal the solution result
	solutionResultBytes, err := json.Marshal(responseMessage)
	if err != nil {
		return err
	}

	err = ch.Publish(
		"",               // exchange
		msg.ReplyTo, // routing key (queue name)
		false,            // mandatory
		false,            // immediate
		amqp.Publishing{
			ContentType: "application/json",
			Body:        solutionResultBytes,
		})
	if err != nil {
		return err
	}

	return nil
}

// Handle errors and requeue the message if needed
func handleError(queueMessage QueueMessage, msg *amqp.Delivery, ch *amqp.Channel, err error, logger *log.Entry) {

	logger.Errorf("Error processing message [MsgID: %s]: %s", queueMessage.MessageID, err)


	newMsg := getNewMsg(msg)
	if newMsg.Body == nil {
		logger.Infof("Dropping message [MsgID: %s] after 3 retries", queueMessage.MessageID)

		failedSolutionResult := solution.SolutionResult{
			Success:    false,
			StatusCode: solution.InternalError,
			Code:       "500",
			Message:    "Failed to process the message after 3 retries: " + err.Error(),
			TestResults: nil,
		}

		// Send the response message to the backend
		err = sendResponseMessage(queueMessage, failedSolutionResult, msg, ch)
		if err != nil {
			logger.Errorf("Failed to send response message [MsgID: %s] to the backend: %s", queueMessage.MessageID, err)
		}
		// Message was retried maxRetries times, ack it and remove it from the queue
		msg.Ack(false)
		return
	}

	// The original message was not processed successfully, acknowledge it and send an updated message to the queue
	msg.Ack(false)

	logger.Infof("Requeuing message [MsgID: %s]", queueMessage.MessageID)

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
			ReplyTo:     newMsg.ReplyTo,
		})
	if err != nil {
		logger.Errorf("Failed to send message [MsgID: %s] to the queue: %s", queueMessage.MessageID, err)
	}
}

// Get the a new messsage to be requeued
func getNewMsg(msg *amqp.Delivery) *amqp.Delivery {
	oldHeaderCount := msg.Headers["x-retry-count"]

	if oldHeaderCount.(int64) >= maxRetries {
		return &amqp.Delivery{}
	}
	newHeaderCount := oldHeaderCount.(int64) + 1

	newMsg := amqp.Delivery{Body: msg.Body, Headers: amqp.Table{"x-retry-count": newHeaderCount}, ReplyTo: string(msg.ReplyTo)}

	return &newMsg
}

// Run the solution using the solution runner
func runSolution(task TaskForRunner, messageID string) (solution.SolutionResult, error) {
	runner := solution.Runner{}


	langConfig := solution.LanguageConfig{
		Type:    task.LanguageType,
		Version: task.LanguageVersion,

	}

	solution := solution.Solution{
		Language:         langConfig,
		BaseDir:          task.TaskDir,
		SolutionFileName: task.SolutionFileName,
		InputDir:         task.InputDirName,
		OutputDir:        task.OutputDirName,
	}

	solutionResult := runner.RunSolution(&solution, messageID)

	return solutionResult, nil
}

// storeSolutionResult sends a POST request with form data including a tar.gz archive.
func storeSolutionResult(solutionResult solution.SolutionResult,task TaskForRunner, queueMessage QueueMessage) error {
	requestURL := "http://host.docker.internal:8080/storeOutputs"
	outputsFolderPath := task.TaskDir + "/" + solutionResult.OutputDir

	// Move the compile error file to the output folder if the solution failed.
	if (solutionResult.StatusCode == solution.Failed) {
		compilationErrorPath := task.TaskDir + "/" + executor.CompileErrorFileName
		err := os.Rename(compilationErrorPath, outputsFolderPath + "/" + executor.CompileErrorFileName)
		if err != nil {
			return err
		}
	}

	// Remove empty error files from the output folder.
	err := utils.RemoveEmptyErrFiles(outputsFolderPath)
	if err != nil {
		return err
	}

	// Compress the output folder into a tar.gz file.
	archiveFilePath, err := utils.TarGzFolder(outputsFolderPath)
	if err != nil {
		return err
	}

	// Open the tar.gz file for reading.
	file, err := os.Open(archiveFilePath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Create a buffer to store the form data.
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add the userID, taskID, submissionNumber and atchive form fields.
	if err := writer.WriteField("userID", strconv.Itoa(int(queueMessage.UserID))); err != nil {
		return err
	}
	if err := writer.WriteField("taskID", strconv.Itoa(int(queueMessage.TaskID))); err != nil {
		return err
	}
	if err := writer.WriteField("submissionNumber", strconv.Itoa(int(queueMessage.SubmissionNumber))); err != nil {
		return err
	}

	part, err := writer.CreateFormFile("archive", filepath.Base(archiveFilePath))
	if err != nil {
		return err
	}
	if _, err := io.Copy(part, file); err != nil {
		return err
	}

	if err := writer.Close(); err != nil {
		return err
	}

	// Create the HTTP request.
	req, err := http.NewRequest("POST", requestURL, body)
	if err != nil {
		return err
	}

	// Set the content type to multipart/form-data.
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Send the request.
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Check the response status.
	if resp.StatusCode != http.StatusOK {
		bodyBytes := new(bytes.Buffer)
		bodyBytes.ReadFrom(resp.Body)
		return errorFailedToStore
	}

	return nil
}
