package executor

type CommandConfig struct {
	WorkspaceDir       string
	InputDirName       string
	OutputDirName      string
	ExecResultFileName string
	DockerImage        string
	TimeLimits         []int
	MemoryLimits       []int
}

type ExecutionResult struct {
	ExitCode int
	ExecTime float64
}

type Executor interface {
	ExecuteCommand(solutionPath, messageID string, cfg CommandConfig) error
}
