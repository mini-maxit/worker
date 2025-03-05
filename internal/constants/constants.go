package constants

// SolutionResult messages
const (
	SolutionMessageSuccess             = "solution executed successfully"
	SolutionMessageRuntimeError        = "solution execution failed"
	SolutionMessageInvalidLanguageType = "invalid language type supplied"
	SolutionMessageTimeout             = "some test cases failed due to time limit exceeded"
	SolutionMessageMemoryLimitExceeded = "some test cases failed due to memory limit exceeded"
)

// TestResult messages
const (
	TestMessageTimeLimitExceeded   = "time limit exceeded"
	TestMessageMemoryLimitExceeded = "memory limit exceeded"
)

// Worker specific constants
const (
	SolutionFileBaseName = "solution"
	InputDirName         = "inputs"
	OutputDirName        = "outputs"
	MaxRetries           = 2
)

// Language types
const (
	CPP = "cpp"
)

// Language versions
const (
	CPP_11 = "11"
	CPP_14 = "14"
	CPP_17 = "17"
	CPP_20 = "20"
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
