package main

import (
	"fmt"
	"os"
	"time"
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

	current_time := time.Now().Local()
	filename := current_time.Format("logs/log_2006-01-02_15-04-05.txt")

	os.Mkdir("logs", 0777)
	f, err := os.Create(filename)
	if err != nil {
		fmt.Println(err)
		return
	}

	var supervisor Supervisor

	supervisor.Init(config, f)
	defer supervisor.Cleanup()

	var shell Shell
	shell.Init(&supervisor)

	go shell.Loop()

	supervisor.Loop()

	<-shell.closed
}
