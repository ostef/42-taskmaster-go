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

	config, err := ParseConfig("test.yml")
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Printf("%+v\n", *config)

	var supervisor Supervisor
	defer supervisor.DestroyAllTasks()

	supervisor.myConfig = append(supervisor.myConfig, MyTaskConfig{
		name:         "say_hello",
		command:      "echo",
		args:         []string{"hello", "sailor"},
		numProcesses: 3,
	})

	supervisor.myConfig = append(supervisor.myConfig, MyTaskConfig{
		name:         "sleep",
		command:      "sleep",
		args:         []string{"10"},
		numProcesses: 3,
	})

	var shell Shell
	shell.supervisor = &supervisor

	shell.Loop()
}
