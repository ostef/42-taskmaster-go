package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

type Shell struct {
	supervisor *Supervisor
}

func (s *Shell) Loop() {
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("$> ")
		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(line)

		parts := strings.Split(line, " ")

		var args []string
		for _, str := range parts {
			if str != "" {
				args = append(args, str)
			}
		}

		if len(args) == 0 {
			continue
		}

		cmd := args[0]
		args = args[1:]

		switch cmd {
		case "start":
			if len(args) != 1 {
				fmt.Println("Error: Expected 1 argument for 'start' command")
				continue
			}

			name := args[0]
			s.supervisor.StartTask(name)

		case "stop":
			if len(args) != 1 {
				fmt.Println("Error: Expected 1 argument for 'stop' command")
				continue
			}

			name := args[0]
			s.supervisor.StopTask(name)

		case "restart":
			if len(args) != 1 {
				fmt.Println("Error: Expected 1 argument for 'restart' command")
				continue
			}

			name := args[0]
			s.supervisor.RestartTask(name)

		case "status":
			if len(args) != 0 {
				fmt.Println("Error: Expected 0 argument for 'status' command")
				continue
			}

			s.supervisor.PrintStatus()

		case "exit":
			if len(args) != 0 {
				fmt.Println("Error: Expected 0 argument for 'exit' command. Exiting anyways")
			}

			return

		default:
			fmt.Printf("Unknown command '%v'\n", cmd)
		}
	}
}
