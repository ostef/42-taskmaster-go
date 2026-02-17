package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: taskmaster <config.yml>")
		os.Exit(1)
	}

	config, err := ParseConfig(os.Args[1])
	if err != nil {
		fmt.Println(err)
		return
	}

	var supervisor Supervisor
	supervisor.Init(config)
	defer supervisor.DestroyAllTasks()

	var shell Shell
	shell.supervisor = &supervisor

	shell.Loop()
}
