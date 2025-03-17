package constants

// Queue message types
const (
	QueueMessageTypeTask      = "task"
	QueueMessageTypeHandshake = "handshake"
	QueueMessageTypeStatus    = "status"
)

// Queue constants
const (
	WorkerQueueName = "worker_queue"
	MaxWorkers      = 10
)

// SolutionResult messages
const (
	SolutionMessageSuccess             = "solution executed successfully"
	SolutionMessageRuntimeError        = "solution execution failed"
	SolutionMessageInvalidLanguageType = "invalid language type supplied"
	SolutionMessageTimeout             = "some test cases failed due to time limit exceeded"
	SolutionMessageMemoryLimitExceeded = "some test cases failed due to memory limit exceeded"
	SolutionMessageTestFailed          = "some test cases failed"
	SolutionMessageLimitsMismatch      = "time and memory limits mismatch compared to the number of test cases"
	SolutionMessageInternalError       = "internal error occurred"
)

// TestResult messages
const (
	TestMessageTimeLimitExceeded   = "time limit exceeded"
	TestMessageMemoryLimitExceeded = "memory limit exceeded"
)

const (
	ExitCodeDifference = 1
)

// Worker specific constants
const (
	SolutionFileBaseName = "solution"
	InputDirName         = "inputs"
	OutputDirName        = "outputs"
	WorkerStatusIdle     = "idle"
	WorkerStatusBusy     = "busy"
)

// Exit codes
const (
	ExitCodeSuccess             = 0
	ExitCodeInternalError       = 1
	ExitCodeTimeLimitExceeded   = 124
	ExitCodeMemoryLimitExceeded = 137
)

// Configuratioin constants
const (
	DefailtRabbitmqHost     = "localhost"
	DefaultRabbitmqUser     = "guest"
	DefaultRabbitmqPassword = "guest"
	DefaultRabbitmqPort     = "5672"
	DefaultFileStorageHost  = "file-storage"
	DefaultFilesStoragePort = "8888"
	DefaultLogPath          = "./internal/logger/logs/log.txt"
	CompileErrorFileName    = "compile-err.err"
	BaseChrootDir           = "../tmp/chroot"
)
