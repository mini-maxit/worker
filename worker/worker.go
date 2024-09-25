package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	amqp "github.com/rabbitmq/amqp091-go"
	_ "github.com/lib/pq"
	"github.com/joho/godotenv"
)

// Task represents the data needed to run a solution
type Task struct {
    fileToExecute  string 
    compilerType   string 
	timeLimit      int
	memoryLimit    int 
    stdin          string 
    expectedStdout string 
}

//message received from the queue
type Solution struct {
	Id              int `json:"id"`
	TaskId          int `json:"task_id"`
	UserId          int `json:"user_id"`
	UserSolutionId  int `json:"user_solution_id"`
	InputOutputId 	    int `json:"input_output_id"` //id of the input entry in the inputs table
}

type SolutionResult struct {
	Id          int `json:"id"`
	UserSolutionId  int `json:"user_solution_id"`
	StatusCode  int `json:"status_code"`
	Message    string `json:"message"`
}
func failOnError(err error, msg string) {
	if err != nil {
		log.Fatalf("%s: %s", msg, err)
	}
}

func main() {
	// Load the environment variables
	err := godotenv.Load()
	if err != nil {
		log.Fatal(err)
	}


	work()
}


func work() {
	// Connect to the database
	DATABASE_USER := os.Getenv("DATABASE_USER")
	DATABASE_PASSWORD := os.Getenv("DATABASE_PASSWORD")
	DATABASE_NAME := os.Getenv("DATABASE_NAME")
	DATABASE_SSL_MODE := os.Getenv("DATABASE_SSL_MODE")

	connStr := fmt.Sprintf("user=%s password=%s dbname=%s sslmode=%s", DATABASE_USER, DATABASE_PASSWORD, DATABASE_NAME, DATABASE_SSL_MODE)

	db, err := sql.Open("postgres", connStr)
	failOnError(err, "Failed to connect to the database")
	defer db.Close()

	// Check if the connection is successful
	err = db.Ping()
	failOnError(err, "Failed to connect to the database")

	log.Printf("Successfully connected to the database!")

	// Connect to the message broker
	RABBITMQ_URL := os.Getenv("RABBITMQ_URL")
	conn, err := amqp.Dial(RABBITMQ_URL)
    failOnError(err, "Failed to connect to RabbitMQ")
    defer conn.Close()

	// Create a channel
    ch, err := conn.Channel()
    failOnError(err, "Failed to open a channel")
    defer ch.Close()

	// Declare a queue
	q, err := ch.QueueDeclare(
		"worker_queue", // name
		false,   // durable
		false,   // delete when unused
		false,   // exclusive
		false,   // no-wait
		nil,     // arguments
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

func processMessage(msg amqp.Delivery, db *sql.DB) {
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


// GetDataForSolutionRunner retrieves the data needed to run the solution
func getDataForSolutionRunner(db *sql.DB, task_id, user_id, user_solution_id, input_output_id int) (Task, error) {
	var task Task
	var dirPath string
	//from tasks table get the directory path
	query := `
		SELECT dir_path
		FROM tasks
		WHERE id = $1
	`
	err := db.QueryRow(query, task_id).Scan(&dirPath)
	if err != nil {
		return Task{}, err
	}

	//from user_solution table get the compiler type
	query = `
		SELECT compiler_type
		FROM user_solutions
		WHERE id = $1
	`
	err = db.QueryRow(query, user_solution_id).Scan(&task.compilerType)
	if err != nil {
		return Task{}, err
	}
	
	//From the input table get the time and memory limits
	query = `
		SELECT time_limit, memory_limit, input_file_path, output_file_path
		FROM inputs_outputs
		WHERE id = $1
	`
	var input_path, output_path string

	err = db.QueryRow(query, input_output_id).Scan(&task.timeLimit, &task.memoryLimit, &input_path, &output_path)
	if err != nil {
		return Task{}, err
	}

	err = getFiles(dirPath, user_id, user_solution_id, input_path, output_path, &task)
	if err != nil {
		return Task{}, err
	}

	return task, nil
}


// GetFiles retrieves the solution, input and output files from the file system
func getFiles(dir_path string, user_id, user_solution_id int, input_path, output_path string ,task *Task) (error) {

	// Get the solution file name
	path := fmt.Sprintf("%s/submissions/user%d/submition%d", dir_path, user_id, user_solution_id)

    // Get the entries in the solution directory
    entries, err := os.ReadDir(path)
    if err != nil {
        return err
    }

    // Filter for files only
    var files []os.DirEntry
    for _, entry := range entries {
        if !entry.IsDir() && entry.Name() != ".DS_Store" {
            files = append(files, entry)
        }
	}


    // Check if there is exactly one file in the solution directory
    if len(files) != 1 {
        return errors.New("expected exactly one file in the solution directory")
    }

	//Construct the paths to the solution
	solutionPath := fmt.Sprintf("%s/submissions/user%d/submition%d/%s", dir_path, user_id, user_solution_id, files[0].Name())
	

	// Read solution file
	solutionContent, err := os.ReadFile(solutionPath)
	if err != nil {
		return err
	}
	task.fileToExecute = string(solutionContent)


	// Read input file
	inputContent, err := os.ReadFile(input_path)
	if err != nil {
		return err
	}
	task.stdin = string(inputContent)

	// Read output file
	outputContent, err := os.ReadFile(output_path)
	if err != nil {
		return err
	}
	task.expectedStdout = string(outputContent)

	return nil
}

// Create a new solution in the database marked as processing
func createSolution(db *sql.DB, solution *Solution) error {
	query := `
		INSERT INTO solutions (task_id, status)
		VALUES ($1, 'processing')
		RETURNING id
	`
	err := db.QueryRow(query, solution.TaskId).Scan(&solution.Id)
	
	return err
}


// runs the solution and returns the result
func runSolution(task Task, user_solution_id int) (SolutionResult, error) {
	//TODO: Implement this function
	//For now, return a dummy result
	exampleSolutionResult := SolutionResult{
		UserSolutionId: user_solution_id,
		StatusCode: 0,
		Message: "Success",
	}

	return exampleSolutionResult ,nil
}


//updates the status of the solution to complete
func markSolutionComplete(db *sql.DB, solution_id int) error {
	_, err := db.Exec("UPDATE solutions SET status = 'complete', checked_at = NOW() WHERE id = $1", solution_id)
	if err != nil {
		return err
	}
	return nil
}

//stores the result of the solution in the database 
func storeSolutionResult(db *sql.DB, solutionResult SolutionResult) error {
	_, err := db.Exec("INSERT INTO user_solution_results (user_solution_id, status_code, message) VALUES ($1, $2, $3)", solutionResult.UserSolutionId, solutionResult.StatusCode, solutionResult.Message)
	return err
}
