package main

import (
	"fmt"

	"github.com/HermanPlay/maxit-worker/executor"
)

func main() {
	executorConfig := executor.NewDefaultExecutorConfig()
	executor := executor.NewDefaultExecutor(executorConfig)

	fmt.Print(executor.ExecuteCommand("ls", ""))

}
