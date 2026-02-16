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
