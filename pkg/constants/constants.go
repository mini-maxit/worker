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
	SolutionMessageTimeout             = "time limit exceeded"
	SolutionMessageMemoryLimitExceeded = "memory limit exceeded"
	SolutionMessageOutputDifference    = "output difference"
	SolutionMessageInternalError       = "internal error occurred"
)

// TestCaseResult messages.
const (
	TestCaseMessageTimeOut             = "Solution timed out after %d ms"
	TestCaseMessageMemoryLimitExceeded = "Solution exceeded memory limit of %d kb"
	TestCaseMessageRuntimeError        = "Solution encountered a runtime error"
)

const (
	ExitCodeDifference = 1
)

// Worker specific constants.
type WorkerStatus int

const (
	WorkerStatusIdle WorkerStatus = iota
	WorkerStatusBusy
)

// Exit codes.
const (
	ExitCodeSuccess             = 0
	ExitCodeTimeLimitExceeded   = 143
	ExitCodeMemoryLimitExceeded = 134
)

// Configuration constants.
const (
	DefaultRabbitmqHost            = "localhost"
	DefaultRabbitmqUser            = "guest"
	DefaultRabbitmqPassword        = "guest"
	DefaultRabbitmqPort            = "5672"
	DefaultStorageHost             = "file-storage"
	DefaultStoragePort             = "8888"
	DefaultLogPath                 = "./internal/logger/logs/log.txt"
	DefaultWorkerQueueName         = "worker_queue"
	DefaultRabbitmqPublishChanSize = 100
	DefaultMaxWorkers              = 10
	RuntimeImagePrefix             = "ghcr.io/mini-maxit/runtime"
	ContainerMaxRunTime            = 30
	DefaultJobsDataVolume          = "maxit_worker_jobs-data"
	DefaultVerifierFlags           = "-w"
)

// Solution package and temporary directory paths.
const (
	TmpDirPath             = "/tmp"
	InputDirName           = "inputs"
	OutputDirName          = "outputs"
	UserOutputDirName      = "userOutputs"
	UserErrorDirName       = "userErrors"
	UserDiffDirName        = "userDiff"
	UserExecResultDirName  = "userExecResults"
	CompileErrFileName     = "compile.err"
	ExecutionResultFileExt = "res"
)

// Docker execution constants.
const (
	MinContainerMemoryKB int64 = 64 * 1024 // 64 MB
	DockerTestScript           = "run_tests.sh"
)

// RabbitMQ specific constants.
const (
	RabbitMQReconnectTries  = 10
	RabbitMQMaxPriority     = 3
	RabbitMQRequeuePriority = 2
)
