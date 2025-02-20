package worker

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/mini-maxit/worker/executor"
	"github.com/mini-maxit/worker/internal/config"
	"github.com/mini-maxit/worker/logger"
	"github.com/mini-maxit/worker/solution"
	"github.com/mini-maxit/worker/utils"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

type Worker struct {
	logger         *zap.SugaredLogger
	ch             *amqp.Channel
	fileStorageUrl string
}

type QueueMessage struct {
	MessageID        string `json:"message_id"`
	TaskID           int64  `json:"task_id"`
	UserID           int64  `json:"user_id"`
	SubmissionNumber int64  `json:"submission_number"`
	LanguageType     string `json:"language_type"`
	LanguageVersion  string `json:"language_version"`
	TimeLimits       []int  `json:"time_limits"`
	MemoryLimits     []int  `json:"memory_limits"`
}

type ResponseMessage struct {
	MessageID string                  `json:"message_id"`
	Result    solution.SolutionResult `json:"result"`
}

const solutionFileBaseName = "solution"

const inputDirName = "inputs"

const outputDirName = "outputs"

const maxRetries = 2

var errorFailedToStore = errors.New("failed to store the solution result")

func NewWorker(conn *amqp.Connection, envConfig *config.Config) *Worker {
	ch := NewRabbitMQChannel(conn)
	return &Worker{
		ch:             ch,
		fileStorageUrl: envConfig.FileStorageUrl,
		logger:         logger.NewNamedLogger("worker"),
	}
}

func (w *Worker) Work() {

	defer func() {
		w.logger.Info("Worker exiting. Closing the channel")
		w.ch.Close()
	}()

	w.logger.Info("Worker started")

	// Declare a queue
	q, err := w.ch.QueueDeclare(
		"worker_queue", // name
		true,           // durable
		false,          // delete when unused
		false,          // exclusive
		false,          // no-wait
		nil,            // arguments
	)
	if err != nil {
		w.logger.Fatalf("Failed to declare a queue: %s", err)
	}

	w.logger.Info("Queue declared")

	// Consume messages from the queue
	msgs, err := w.ch.Consume(
		q.Name, // queue
		"",     // consumer
		false,  // auto-ack to be able to handle errors and requeue
		false,  // exclusive
		false,  // no-local
		false,  // no-wait
		nil,    // args
	)
	if err != nil {
		w.logger.Fatalf("Failed to register a consumer: %s", err)
	}

	var forever = make(chan struct{})

	go func() {
		for msg := range msgs {
			func(msg amqp.Delivery) {
				var queueMessage QueueMessage

				w.logger.Info("Received a message")

				err := json.Unmarshal(msg.Body, &queueMessage)
				if err != nil {
					w.handleError(QueueMessage{}, &msg, err)
				}

				w.logger.Infof("Processing message [MsgID: %s]", queueMessage.MessageID)

				defer func() {
					if r := recover(); r != nil {
						err := msg.Ack(false)
						if err != nil {
							w.logger.Errorf("Failed to acknowledge message [MsgID: %s]: %s", queueMessage.MessageID, err)
						}
						w.logger.Errorf("Recovered from panic: %v", r)
					}
				}()

				err = w.processMessage(queueMessage, &msg)
				if err != nil {
					w.handleError(queueMessage, &msg, err)
				} else {
					err := msg.Ack(false)
					if err != nil {
						w.logger.Errorf("Failed to acknowledge message [MsgID: %s]: %s", queueMessage.MessageID, err)
					}
				}

				w.logger.Infof("Processed message [MsgID: %s]", queueMessage.MessageID)
			}(msg)
		}
	}()

	w.logger.Info("[*] Waiting for messages")
	<-forever
}

// Process the incoming message
func (w *Worker) processMessage(queueMessage QueueMessage, msg *amqp.Delivery) error {

	w.logger.Infof("Getting data for solution runner [MsgID: %s]", queueMessage.MessageID)

	solutionData := NewSolutionData(queueMessage.TaskID, queueMessage.UserID, queueMessage.SubmissionNumber)

	task, err := solutionData.getDataForSolutionRunner(w.fileStorageUrl)
	if err != nil {
		return err
	}

	defer func() {
		err := utils.RemoveIO(task.TempDir, true, true)
		if err != nil {
			w.logger.Errorf("Failed to remove temp directory: %s", err)
		}
	}()

	task.LanguageType, err = solution.StringToLanguageType(queueMessage.LanguageType)
	if err != nil {
		return err
	}

	task.SolutionFileName, err = solution.GetSolutionFileNameWithExtension(solutionFileBaseName, task.LanguageType)
	if err != nil {
		return err
	}

	task.LanguageVersion = queueMessage.LanguageVersion
	task.TimeLimits = queueMessage.TimeLimits
	task.MemoryLimits = queueMessage.MemoryLimits
	task.InputDirName = inputDirName
	task.OutputDirName = outputDirName

	w.logger.Infof("Data for solution runner retrieved [MsgID: %s]", queueMessage.MessageID)

	w.logger.Infof("Running solution [MsgID: %s]", queueMessage.MessageID)

	solutionResult := runSolution(task, queueMessage.MessageID)

	w.logger.Infof("Storing solution result [MsgID: %s]", queueMessage.MessageID)

	err = w.storeSolutionResult(solutionResult, task, queueMessage, w.fileStorageUrl)
	if err != nil {
		return err
	}

	w.logger.Infof("Sending response message [MsgID: %s]", queueMessage.MessageID)

	err = w.sendResponseMessage(queueMessage, solutionResult, msg)
	if err != nil {
		return err
	}

	w.logger.Infof("Response message sent [MsgID: %s]", queueMessage.MessageID)

	return nil
}

// Send response message to backend with solution result
func (w *Worker) sendResponseMessage(queueMessage QueueMessage, solutionResult solution.SolutionResult, msg *amqp.Delivery) error {

	responseMessage := ResponseMessage{
		MessageID: queueMessage.MessageID,
		Result:    solutionResult,
	}

	solutionResultBytes, err := json.Marshal(responseMessage)
	if err != nil {
		return err
	}

	err = w.ch.Publish(
		"",          // exchange
		msg.ReplyTo, // routing key (queue name)
		false,       // mandatory
		false,       // immediate
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
func (w *Worker) handleError(queueMessage QueueMessage, msg *amqp.Delivery, err error) {

	w.logger.Errorf("Error processing message [MsgID: %s]: %s", queueMessage.MessageID, err)

	newMsg := getNewMsg(msg)
	if newMsg.Body == nil {
		w.logger.Infof("Dropping message [MsgID: %s] after 3 retries", queueMessage.MessageID)

		failedSolutionResult := solution.SolutionResult{
			Success:     false,
			StatusCode:  solution.InternalError,
			Code:        solution.InternalError.String(),
			Message:     "Failed to process the message after 3 retries: " + err.Error(),
			TestResults: nil,
		}

		err = w.sendResponseMessage(queueMessage, failedSolutionResult, msg)
		if err != nil {
			w.logger.Errorf("Failed to send response message [MsgID: %s] to the backend: %s", queueMessage.MessageID, err)
		}

		err = msg.Ack(false)
		if err != nil {
			w.logger.Errorf("Failed to acknowledge message [MsgID: %s]: %s", queueMessage.MessageID, err)
		}
		return
	}

	err = msg.Ack(false)
	if err != nil {
		w.logger.Errorf("Failed to acknowledge message [MsgID: %s]: %s", queueMessage.MessageID, err)
	}

	w.logger.Infof("Requeuing message [MsgID: %s]", queueMessage.MessageID)

	err = w.ch.Publish(
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
		w.logger.Errorf("Failed to send message [MsgID: %s] to the queue: %s", queueMessage.MessageID, err)
	}
}

func getNewMsg(msg *amqp.Delivery) *amqp.Delivery {
	oldHeaderCount := msg.Headers["x-retry-count"]

	if oldHeaderCount.(int32) >= maxRetries {
		return &amqp.Delivery{}
	}
	newHeaderCount := oldHeaderCount.(int32) + 1

	// Update new messege header
	newMsg := amqp.Delivery{Body: msg.Body, Headers: amqp.Table{"x-retry-count": newHeaderCount}, ReplyTo: string(msg.ReplyTo)}

	return &newMsg
}

func runSolution(task TaskForRunner, messageID string) solution.SolutionResult {
	runner := solution.NewRunner()

	langConfig := solution.LanguageConfig{
		Type:    task.LanguageType,
		Version: task.LanguageVersion,
	}

	solution := solution.Solution{
		Language:         langConfig,
		TimeLimits:       task.TimeLimits,
		MemoryLimits:     task.MemoryLimits,
		BaseDir:          task.TaskDir,
		SolutionFileName: task.SolutionFileName,
		InputDir:         task.InputDirName,
		OutputDir:        task.OutputDirName,
	}

	solutionResult := runner.RunSolution(&solution, messageID)

	return solutionResult
}

func (w *Worker) storeSolutionResult(solutionResult solution.SolutionResult, task TaskForRunner, queueMessage QueueMessage, fileStorageUrl string) error {
	requestURL := fmt.Sprintf("%s/storeOutputs", fileStorageUrl)
	outputsFolderPath := task.TaskDir + "/" + solutionResult.OutputDir

	if solutionResult.StatusCode == solution.CompilationError {
		compilationErrorPath := task.TaskDir + "/" + executor.CompileErrorFileName
		err := os.Rename(compilationErrorPath, outputsFolderPath+"/"+executor.CompileErrorFileName)
		if err != nil {
			return err
		}
	}

	err := utils.RemoveEmptyErrFiles(outputsFolderPath)
	if err != nil {
		return err
	}

	archiveFilePath, err := utils.TarGzFolder(outputsFolderPath)
	if err != nil {
		return err
	}

	file, err := os.Open(archiveFilePath)
	if err != nil {
		return err
	}
	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

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

	req, err := http.NewRequest("POST", requestURL, body)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes := new(bytes.Buffer)
		_, err := bodyBytes.ReadFrom(resp.Body)
		if err != nil {
			w.logger.Errorf("Failed to read response body: %s", err)
		}
		w.logger.Errorf("Failed to store the solution result: %s", bodyBytes.String())
		return errorFailedToStore
	}

	return nil
}
