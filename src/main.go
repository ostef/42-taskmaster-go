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
	defer supervisor.DestroyAllTasks()

	supervisor.Init(config)

	var shell Shell
	shell.Init(&supervisor)

	go shell.Loop()

	supervisor.Loop()

	<-shell.closed
}
