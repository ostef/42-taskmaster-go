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
	err   error
}

func (s *ProcessStatus) Set(status uint, err error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.value = status
	s.err = err
}

func (s *ProcessStatus) Get() (uint, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	return s.value, s.err
}

func (s *ProcessStatus) String() string {
	status, err := s.Get()

	switch status {
	case ProcessStatusNotStarted:
		return "not started"
	case ProcessStatusStarted:
		return "started"
	case ProcessStatusRunning:
		return "running"
	case ProcessStatusStopping:
		return "stopping"
	case ProcessStatusStopped:
		return fmt.Sprintf("stopped (%v)", err)
	case ProcessStatusKilled:
		return fmt.Sprintf("killed (%v)", err)
	case ProcessStatusError:
		return fmt.Sprintf("error (%v)", err)
	default:
		return fmt.Sprintf("??? (%v)", err)
	}
}

const (
	ProcessStatusNotStarted = iota
	ProcessStatusStarted    = iota
	ProcessStatusRunning    = iota
	ProcessStatusStopping   = iota
	ProcessStatusStopped    = iota
	ProcessStatusKilled     = iota
	ProcessStatusError      = iota // This means the process goroutine has stopped
)

type ProcessCommand uint

const (
	ProcessCommandStop    = iota
	ProcessCommandRestart = iota
	ProcessCommandDestroy = iota
)

type TaskProcess struct {
	status       ProcessStatus
	cmd          *exec.Cmd
	commandQueue chan ProcessCommand
	config       TaskConfig
	configMutex  sync.RWMutex
}

func (p *TaskProcess) getConfig() TaskConfig {
	p.configMutex.Lock()
	defer p.configMutex.Unlock()

	cfg := p.config

	return cfg
}

func (p *TaskProcess) setConfig(cfg TaskConfig) {
	p.configMutex.Lock()
	defer p.configMutex.Unlock()

	p.config = cfg
}

type Task struct {
	name      string
	processes []*TaskProcess
}

type Supervisor struct {
	waitGroup sync.WaitGroup
	tasks     []*Task
	config    Config
}

func (s *Supervisor) getTaskConfig(name string) *TaskConfig {
	idx := slices.IndexFunc(s.config.tasks, func(t TaskConfig) bool { return t.Name == name })
	if idx < 0 {
		return nil
	}

	return &s.config.tasks[idx]
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
		return fmt.Errorf("No task named '%v'", name)
	}

	task := s.getTask(name)
	if task == nil {
		task = new(Task)
		s.tasks = append(s.tasks, task)

		task.name = name
	}

	for _, process := range task.processes {
		status, _ := process.status.Get()
		if status == ProcessStatusStarted || status == ProcessStatusRunning || status == ProcessStatusStopping {
			return fmt.Errorf("Task '%v' still has some running processes", name)
		}
	}

	for _, process := range task.processes {
		status, _ := process.status.Get()
		if status == ProcessStatusError {
			process.setConfig(*config)

			s.waitGroup.Go(func() { process.Run(context.Background()) })
		} else {
			process.commandQueue <- ProcessCommandRestart
		}
	}

	numNewProcessesToSpawn := config.NumProcesses - len(task.processes)
	for range numNewProcessesToSpawn {
		process := new(TaskProcess)
		task.processes = append(task.processes, process)

		process.config = *config
		process.commandQueue = make(chan ProcessCommand, 3)

		s.waitGroup.Go(func() { process.Run(context.Background()) })
	}

	return nil
}

func (s *Supervisor) StopTask(name string) error {
	task := s.getTask(name)
	if task == nil {
		return fmt.Errorf("No task named '%v'", name)
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

	config := s.getTaskConfig(name)
	if config == nil {
		return fmt.Errorf("No task named '%v'", name)
	}

	for _, process := range task.processes {
		status, _ := process.status.Get()
		if status == ProcessStatusError {
			process.setConfig(*config)

			s.waitGroup.Go(func() { process.Run(context.Background()) })
		} else {
			process.commandQueue <- ProcessCommandRestart
		}
	}

	return nil
}

func (s *Supervisor) DestroyAllTasks() error {
	for _, task := range s.tasks {
		for _, process := range task.processes {
			process.commandQueue <- ProcessCommandDestroy
		}
	}

	s.waitGroup.Wait()

	s.tasks = nil

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
		p.status.Set(ProcessStatusStopped, err)
		return err

	case <-time.After(2 * time.Second):
		fmt.Println("Process still exiting, sending SIGKILL...")
		_ = p.cmd.Process.Kill()
		err := <-done
		fmt.Println("Process killed:", err)
		p.status.Set(ProcessStatusKilled, err)

		return err
	}
}

func (p *TaskProcess) Run(ctx context.Context) error {
	for true {
		config := p.getConfig()

		fmt.Printf("Starting process for %v\n", config.Name)

		p.cmd = exec.CommandContext(context.Background(), config.Command, config.Args...)
		p.cmd.Stdout = os.Stdout
		p.cmd.Stderr = os.Stderr

		err := p.cmd.Start()
		if err != nil {
			fmt.Println("Could not start process:", err)
			p.status.Set(ProcessStatusError, err)

			return err
		}

		p.status.Set(ProcessStatusStarted, nil)

		done := make(chan error, 1)
		shouldRestart := false

		go func() {
			done <- p.cmd.Wait()
		}()

		// Process startup
		select {
		case err := <-done:
			p.status.Set(ProcessStatusStopped, err)
			fmt.Println("Process exited early:", err)

		case request := <-p.commandQueue:
			switch request {
			case ProcessCommandDestroy:
				return p.TerminateOrKill(done)

			case ProcessCommandStop:
				p.TerminateOrKill(done)

			case ProcessCommandRestart:
				shouldRestart = true
				p.TerminateOrKill(done)
			}

		case <-time.After(2 * time.Second):
			p.status.Set(ProcessStatusRunning, nil)
			fmt.Println("Process has sucessfully started")
		}

		config = p.getConfig()

		// Process is running
		select {
		case err := <-done:
			p.status.Set(ProcessStatusStopped, err)
			fmt.Println("Process exited:", err)

		case request := <-p.commandQueue:
			switch request {
			case ProcessCommandDestroy:
				return p.TerminateOrKill(done)

			case ProcessCommandStop:
				p.TerminateOrKill(done)

			case ProcessCommandRestart:
				shouldRestart = true
				p.TerminateOrKill(done)
			}
		}

		for !shouldRestart {
			request := <-p.commandQueue
			switch request {
			case ProcessCommandRestart:
				shouldRestart = true
			case ProcessCommandDestroy:
				return nil
			}
		}
	}

	return nil
}
