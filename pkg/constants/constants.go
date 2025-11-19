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
	TestCaseMessageTimeOut             = "Solution timed out after %d s"
	TestCaseMessageMemoryLimitExceeded = "Solution exceeded memory limit of %d MB"
	TestCaseMessageRuntimeError        = "Solution encountered a runtime error"
)

const (
	ExitCodeDifference = 1
)

// WorkerStatus represents the status of a worker.
type WorkerStatus int

// Worker status enum values.
const (
	WorkerStatusIdle WorkerStatus = iota
	WorkerStatusBusy
)

// String returns the string representation of the WorkerStatus.
func (s WorkerStatus) String() string {
	switch s {
	case WorkerStatusIdle:
		return "idle"
	case WorkerStatusBusy:
		return "busy"
	default:
		return "unknown"
	}
}

// MarshalJSON implements json.Marshaler to serialize WorkerStatus as a string.
func (s WorkerStatus) MarshalJSON() ([]byte, error) {
	return []byte(`"` + s.String() + `"`), nil
}

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
	RuntimeImagePrefix             = "seber/runtime"
	ContainerMaxRunTime            = 30
	DefaultJobsDataVolume          = "maxit_worker_jobs-data"
	DefaultVerifierFlags           = "-w"
)

// Solution package and temporary directory paths.
const (
	TmpDirPath             = "/tmp/"
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
