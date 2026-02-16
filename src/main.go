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

	fmt.Printf("%+v\n", config)

	var supervisor Supervisor
	defer supervisor.DestroyAllTasks()

	supervisor.config = config
	// supervisor.myConfig = append(supervisor.myConfig, MyTaskConfig{
	// 	Name:         "say_hello",
	// 	Command:      "echo",
	// 	Args:         []string{"hello", "sailor"},
	// 	NumProcesses: 3,
	// })

	// supervisor.myConfig = append(supervisor.myConfig, MyTaskConfig{
	// 	Name:         "sleep",
	// 	Command:      "sleep",
	// 	Args:         []string{"10"},
	// 	NumProcesses: 3,
	// })

	var shell Shell
	shell.supervisor = &supervisor

	shell.Loop()
}
