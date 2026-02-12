package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"sync"
	"syscall"
	"time"
)

type ProcessStatus struct {
	mutex sync.RWMutex
	value uint
}

func (s *ProcessStatus) Set(status uint) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.value = status
}

func (s *ProcessStatus) Get() uint {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	return s.value
}

func (s *ProcessStatus) String() string {
	status := s.Get()

	switch status {
	case ProcessStatusStarted:
		return "started"
	case ProcessStatusRunning:
		return "running"
	case ProcessStatusStopping:
		return "stopping"
	case ProcessStatusStopped:
		return "stopped"
	case ProcessStatusKilled:
		return "killed"
	default:
		return "???"
	}
}

const (
	ProcessStatusStarted  = iota
	ProcessStatusRunning  = iota
	ProcessStatusStopping = iota
	ProcessStatusStopped  = iota
	ProcessStatusKilled   = iota
)

type ProcessCommand uint

const (
	ProcessCommandStop    = iota
	ProcessCommandRestart = iota
)

type TaskProcess struct {
	status       ProcessStatus
	cmd          *exec.Cmd
	commandQueue chan ProcessCommand
}

type Task struct {
	name      string
	processes []*TaskProcess
}

type MyTaskConfig struct {
	name         string
	command      string
	args         []string
	numProcesses int
}

type Supervisor struct {
	tasks    []*Task
	myConfig []MyTaskConfig
}

func (s *Supervisor) getTaskConfig(name string) *MyTaskConfig {
	idx := slices.IndexFunc(s.myConfig, func(t MyTaskConfig) bool { return t.name == name })
	if idx < 0 {
		return nil
	}

	return &s.myConfig[idx]
}

func (s *Supervisor) getTask(name string) *Task {
	idx := slices.IndexFunc(s.tasks, func(t *Task) bool { return t.name == name })
	if idx < 0 {
		return nil
	}

	return s.tasks[idx]
}

func (s *Supervisor) StartTask(name string) error {
	config := s.getTaskConfig(name)
	if config == nil {
		return fmt.Errorf("Unknown task '%v'", name)
	}

	task := s.getTask(name)
	if task == nil {
		task = new(Task)
		s.tasks = append(s.tasks, task)

		task.name = name
	}

	for _, process := range task.processes {
		status := process.status.Get()
		if status != ProcessStatusStopped && status != ProcessStatusKilled {
			return fmt.Errorf("Task '%v' still has some running processes", name)
		}
	}

	for _, process := range task.processes {
		process.commandQueue <- ProcessCommandRestart
	}

	numNewProcessesToSpawn := config.numProcesses - len(task.processes)
	for range numNewProcessesToSpawn {
		process := new(TaskProcess)
		task.processes = append(task.processes, process)

		process.commandQueue = make(chan ProcessCommand, 3)

		go process.Run(*config)
	}

	return nil
}

func (s *Supervisor) StopTask(name string) error {
	task := s.getTask(name)
	if task == nil {
		return fmt.Errorf("No task '%v'", name)
	}

	for _, process := range task.processes {
		process.commandQueue <- ProcessCommandStop
	}

	return nil
}

func (s *Supervisor) RestartTask(name string) error {
	task := s.getTask(name)
	if task == nil {
		return s.StartTask(name)
	}

	for _, process := range task.processes {
		process.commandQueue <- ProcessCommandRestart
	}

	return nil
}

func (s *Supervisor) StopAllTasks() error {
	for _, task := range s.tasks {
		for _, process := range task.processes {
			process.commandQueue <- ProcessCommandStop
		}
	}

	return nil
}

func (s *Supervisor) PrintStatus() {
	if len(s.tasks) == 0 {
		fmt.Println("No tasks")
		return
	}

	for _, task := range s.tasks {
		fmt.Printf("Task '%v':\n", task.name)

		if len(task.processes) == 0 {
			fmt.Println("No process")
		} else {
			fmt.Printf("Processes (%d):\n", len(task.processes))
			for i, process := range task.processes {
				fmt.Printf("  %d: %v\n", i, process.status.String())
			}
		}
	}
}

func (p *TaskProcess) TerminateOrKill(done chan error) error {
	fmt.Println("Shutting down process...")
	_ = p.cmd.Process.Signal(syscall.SIGTERM)

	select {
	case err := <-done:
		fmt.Println("Process exited gracefully:", err)
		p.status.Set(ProcessStatusStopped)
		return err

	case <-time.After(2 * time.Second):
		fmt.Println("Process still exiting, sending SIGKILL...")
		_ = p.cmd.Process.Kill()
		err := <-done
		fmt.Println("Process killed:", err)
		p.status.Set(ProcessStatusKilled)

		return err
	}
}

func (p *TaskProcess) Run(config MyTaskConfig) error {
	for true {
		fmt.Println("Starting process for", config.name)

		p.cmd = exec.CommandContext(context.Background(), config.command, config.args...)
		p.cmd.Stdout = os.Stdout
		p.cmd.Stderr = os.Stderr

		p.status.Set(ProcessStatusStarted)
		p.cmd.Start()

		done := make(chan error, 1)
		shouldRestart := false

		go func() {
			done <- p.cmd.Wait()
		}()

		// Process startup
		select {
		case request := <-p.commandQueue:
			switch request {
			case ProcessCommandStop:
				p.TerminateOrKill(done)

			case ProcessCommandRestart:
				shouldRestart = true
				p.TerminateOrKill(done)
			}

		case err := <-done:
			p.status.Set(ProcessStatusStopped)
			fmt.Println("Process exited early:", err)
			// return err

		case <-time.After(2 * time.Second):
			p.status.Set(ProcessStatusRunning)
			fmt.Println("Process has sucessfully started")
		}

		// Process is running
		select {
		case request := <-p.commandQueue:
			switch request {
			case ProcessCommandStop:
				p.TerminateOrKill(done)

			case ProcessCommandRestart:
				shouldRestart = true
				p.TerminateOrKill(done)
			}

		case err := <-done:
			p.status.Set(ProcessStatusStopped)
			fmt.Println("Process exited:", err)
			// return err
		}

		for !shouldRestart {
			request := <-p.commandQueue
			if request == ProcessCommandRestart {
				shouldRestart = true
			}
		}
	}

	return nil
}
