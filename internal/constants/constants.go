package constants

// Queue message types.
const (
	QueueMessageTypeTask      = "task"
	QueueMessageTypeHandshake = "handshake"
	QueueMessageTypeStatus    = "status"
)

// SolutionResult messages.
const (
	SolutionMessageSuccess             = "solution executed successfully"
	SolutionMessageRuntimeError        = "solution execution failed"
	SolutionMessageInvalidLanguageType = "invalid language type supplied"
	SolutionMessageTimeout             = "time limit exceeded"
	SolutionMessageMemoryLimitExceeded = "memory limit exceeded"
	SolutionMessageOutputDifference    = "output difference"
	SolutionMessageLimitsMismatch      = "time and memory limits mismatch compared to the number of test cases"
	SolutionMessageInternalError       = "internal error occurred"
)

const (
	ExitCodeDifference = 1
)

// Worker specific constants.
const (
	SolutionFileBaseName = "solution"
	InputDirName         = "inputs"
	OutputDirName        = "outputs"
	WorkerStatusIdle     = "idle"
	WorkerStatusBusy     = "busy"
)

// Exit codes.
const (
	ExitCodeSuccess             = 0
	ExitCodeInternalError       = 1
	ExitCodeTimeLimitExceeded   = 124
	ExitCodeMemoryLimitExceeded = 137
)

// Configuratioin constants.
const (
	DefaultRabbitmqHost     = "localhost"
	DefaultRabbitmqUser     = "guest"
	DefaultRabbitmqPassword = "guest"
	DefaultRabbitmqPort     = "5672"
	DefaultFileStorageHost  = "file-storage"
	DefaultFilesStoragePort = "8888"
	DefaultLogPath          = "./internal/logger/logs/log.txt"
	CompileErrorFileName    = "compile-err.err"
	BaseChrootDir           = "../tmp/chroot"
	DefaultWorkerQueueName  = "worker_queue"
	DefaultMaxWorkersStr    = "10"
	UserOutputDirName       = "user-output"
)

// Utility constants.
const (
	MaxFileSize = 10 * 1024 * 1024 // 10 MB
)
