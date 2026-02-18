package main

import (
	"fmt"
	"strings"

	"github.com/peterh/liner"
)

type Shell struct {
	supervisor *Supervisor
	closed     chan bool
}

type ShellCommand struct {
	Name        string
	Args        []string
	Description string
}

var shell_commands = []ShellCommand{
	{
		Name:        "help",
		Description: "List all commands",
	},
	{
		Name:        "start",
		Args:        []string{"task_name"},
		Description: "Start a non running task",
	},
	{
		Name:        "stop",
		Args:        []string{"task_name"},
		Description: "Stop a running task",
	},
	{
		Name:        "restart",
		Args:        []string{"task_name"},
		Description: "Stop then restart a task",
	},
	{
		Name:        "reload",
		Description: "Reload the config file",
	},
	{
		Name:        "status",
		Description: "Print information about the tasks that have been started",
	},
	{
		Name:        "config",
		Description: "Print the config file",
	},
	{
		Name:        "exit",
		Description: "Shutdown all tasks and quit the program",
	},
}

func (s *Shell) Init(supervisor *Supervisor) {
	s.closed = make(chan bool, 1)
	s.supervisor = supervisor
}

func (s *Shell) PrintHelp() {
	fmt.Println("Commands:")

	const Align int = 20
	for _, cmd := range shell_commands {
		i := 0
		ii := 0
		ii, _ = fmt.Printf("  %v", cmd.Name)
		i += ii

		for _, arg := range cmd.Args {
			ii, _ = fmt.Printf(" %v", arg)
			i += ii
		}

		fmt.Printf(": ")
		for range Align - i {
			fmt.Printf(" ")
		}

		fmt.Printf("%v\n", cmd.Description)
	}
}

func (s *Shell) Loop() {
	defer func() {
		s.closed <- true
	}()

	line := liner.NewLiner()
	defer line.Close()

	line.SetCtrlCAborts(true)
	line.SetCompleter(func(line string) (c []string) {
		for _, cmd := range shell_commands {
			if strings.HasPrefix(cmd.Name, strings.ToLower(line)) {
				c = append(c, cmd.Name)
			}
		}
		return
	})

	errChan := make(chan error, 1)
	for {
		input, err := line.Prompt("$> ")
		if err != nil {
			s.supervisor.commandQueue <- SupervisorCommand{Kind: SupervisorExit, ErrChan: errChan}
			<-errChan

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

			s.supervisor.commandQueue <- SupervisorCommand{Kind: SupervisorStartTask, TaskName: args[0], ErrChan: errChan}
			err := <-errChan

			if err != nil {
				fmt.Println("Error:", err)
			}

		case "stop":
			if len(args) != 1 {
				fmt.Println("Error: Expected 1 argument for 'stop' command")
				continue
			}

			s.supervisor.commandQueue <- SupervisorCommand{Kind: SupervisorStopTask, TaskName: args[0], ErrChan: errChan}
			err := <-errChan

			if err != nil {
				fmt.Println("Error:", err)
			}

		case "restart":
			if len(args) != 1 {
				fmt.Println("Error: Expected 1 argument for 'restart' command")
				continue
			}

			s.supervisor.commandQueue <- SupervisorCommand{Kind: SupervisorRestartTask, TaskName: args[0], ErrChan: errChan}
			err := <-errChan

			if err != nil {
				fmt.Println("Error:", err)
			}

		case "reload":
			if len(args) != 0 {
				fmt.Println("Error: Expected 0 argument for 'reload' command")
				continue
			}

			s.supervisor.commandQueue <- SupervisorCommand{Kind: SupervisorReloadConfig, ErrChan: errChan}
			err := <-errChan

			if err != nil {
				fmt.Println("Error:", err)
			}

		case "status":
			if len(args) != 0 {
				fmt.Println("Error: Expected 0 argument for 'status' command")
				continue
			}

			s.supervisor.commandQueue <- SupervisorCommand{Kind: SupervisorPrintStatus, ErrChan: errChan}
			<-errChan

		case "config":
			if len(args) != 0 {
				fmt.Println("Error: Expected 0 argument for 'config' command")
				continue
			}

			s.supervisor.commandQueue <- SupervisorCommand{Kind: SupervisorPrintConfig, ErrChan: errChan}
			<-errChan

		case "exit":
			if len(args) != 0 {
				fmt.Println("Error: Expected 0 argument for 'exit' command. Exiting anyways")
			}

			s.supervisor.commandQueue <- SupervisorCommand{Kind: SupervisorExit, ErrChan: errChan}
			err := <-errChan

			if err != nil {
				fmt.Println("Error:", err)
			}

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
