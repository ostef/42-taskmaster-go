package main

import (
	"fmt"
	"strings"

	"github.com/peterh/liner"
)

type Shell struct {
	supervisor *Supervisor
}

var commands = []string{"start", "stop", "restart", "reload", "status", "exit", "help"}

func (s *Shell) PrintHelp() {
	fmt.Println("Commands:")
	fmt.Println("  help")
	fmt.Println("  start {task}")
	fmt.Println("  stop {task}")
	fmt.Println("  restart {task}")
	fmt.Println("  reload")
	fmt.Println("  status")
	fmt.Println("  exit")
}

func (s *Shell) Loop() {
	line := liner.NewLiner()
	defer line.Close()

	line.SetCtrlCAborts(true)
	line.SetCompleter(func(line string) (c []string) {
		for _, n := range commands {
			if strings.HasPrefix(n, strings.ToLower(line)) {
				c = append(c, n)
			}
		}
		return
	})

	for {
		input, err := line.Prompt("$> ")
		if err != nil {
			s.supervisor.DestroyAllTasks()
			return
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		line.AppendHistory(input)

		parts := strings.Fields(input)
		cmd := parts[0]
		args := parts[1:]

		switch cmd {
		case "start":
			if len(args) != 1 {
				fmt.Println("Error: Expected 1 argument for 'start' command")
				continue
			}

			name := args[0]
			err := s.supervisor.StartTask(name)

			if err != nil {
				fmt.Println("Error:", err)
			}

		case "stop":
			if len(args) != 1 {
				fmt.Println("Error: Expected 1 argument for 'stop' command")
				continue
			}

			name := args[0]
			err := s.supervisor.StopTask(name)

			if err != nil {
				fmt.Println("Error:", err)
			}

		case "restart":
			if len(args) != 1 {
				fmt.Println("Error: Expected 1 argument for 'restart' command")
				continue
			}

			name := args[0]
			err := s.supervisor.RestartTask(name)

			if err != nil {
				fmt.Println("Error:", err)
			}

		case "reload":
			if len(args) != 0 {
				fmt.Println("Error: Expected 0 argument for 'reload' command")
				continue
			}

			s.supervisor.ReloadConfig()

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

			s.supervisor.DestroyAllTasks()

			return

		case "help":
			if len(args) != 0 {
				fmt.Println("Error: Expected 0 argument for 'help' command")
				continue
			}

			s.PrintHelp()

		default:
			fmt.Printf("Unknown command '%v'\n", cmd)
		}
	}
}
