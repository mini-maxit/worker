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
	SolutionMessageNonZeroExitCode     = "solution returned non-zero exit code"
	SolutionMessageTimeout             = "time limit exceeded"
	SolutionMessageMemoryLimitExceeded = "memory limit exceeded"
	SolutionMessageOutputDifference    = "output difference"
	SolutionMessageInternalError       = "internal error occurred"
	SolutionMessageCompilationError    = "compilation error occurred"
)

// TestCaseResult messages.
const (
	TestCaseMessageTimeOut                = "Solution timed out after %d ms"
	TestCaseMessageMemoryLimitExceeded    = "Solution exceeded memory limit of %d kb"
	TestCaseMessageNonZeroExitCode        = "Solution exited with non-zero exit code %d"
	TestCaseMessagePossiblyExceededMemory = "Solution exit code %d, possibly exceeded memory limit of %d kb"
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
	ExitCodeSuccess           = 0
	ExitCodeTimeLimitExceeded = 143
	ExitCodeCommandNotFound   = 127
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

// Cache configuration.
const (
	CacheDirPath      = "/tmp/worker-cache"
	CacheTTLHours     = 24
	CacheMetadataFile = ".cache_meta.json"
	CacheMaxEntries   = 1000
)

// Docker execution constants.
const (
	MinContainerMemoryKB       int64 = 64 * 1024        // 64 MB
	MaxContainerOutputFileSize int64 = 10 * 1024 * 1024 // 10 MB per output file
	DockerTestScript                 = "run_tests.sh"
	RunnerUID                        = 1000
	RunnerGID                        = 1000
	RunnerName                       = "runner"
)

// RabbitMQ specific constants.
const (
	RabbitMQReconnectTries  = 10
	RabbitMQMaxPriority     = 3
	RabbitMQRequeuePriority = 2
)
