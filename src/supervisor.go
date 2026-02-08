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

type TaskProcess struct {
	status      ProcessStatus
	cmd         *exec.Cmd
	stopRequest chan bool
}

type RunningTask struct {
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
	runningTasks []*RunningTask
	myConfig     []MyTaskConfig
}

func (s *Supervisor) getTaskConfig(name string) *MyTaskConfig {
	idx := slices.IndexFunc(s.myConfig, func(t MyTaskConfig) bool { return t.name == name })
	if idx < 0 {
		return nil
	}

	return &s.myConfig[idx]
}

func (s *Supervisor) getRunningTask(name string) *RunningTask {
	idx := slices.IndexFunc(s.runningTasks, func(t *RunningTask) bool { return t.name == name })
	if idx < 0 {
		return nil
	}

	return s.runningTasks[idx]
}

func (s *Supervisor) StartTask(name string) error {
	config := s.getTaskConfig(name)
	if config == nil {
		return fmt.Errorf("Unknown task '%v'", name)
	}

	running := s.getRunningTask(name)
	if running != nil {
		return fmt.Errorf("Task '%v' is already running", name)
	}

	running = new(RunningTask)
	s.runningTasks = append(s.runningTasks, running)

	running.name = name

	for range config.numProcesses {
		process := new(TaskProcess)
		running.processes = append(running.processes, process)

		process.stopRequest = make(chan bool, 1)

		go process.Run(*config)
	}

	return nil
}

func (s *Supervisor) StopTask(name string) error {
	running := s.getRunningTask(name)
	if running == nil {
		return fmt.Errorf("Task '%v' is not running", name)
	}

	for _, process := range running.processes {
		process.stopRequest <- true
	}

	return nil
}

func (s *Supervisor) RestartTask(name string) error {
	return nil
}

func (s *Supervisor) StopAllTasks() error {
	return nil
}

func (s *Supervisor) PrintStatus() {
	if len(s.runningTasks) == 0 {
		fmt.Println("No running tasks")
		return
	}

	fmt.Println("Status:")
	for _, task := range s.runningTasks {
		fmt.Printf("Task '%v':\n", task.name)
		fmt.Printf("Processes (%d):\n", len(task.processes))
		for i, process := range task.processes {
			fmt.Printf("  %d: %v\n", i, process.status.String())
		}
	}
}

func (p *TaskProcess) SendTermSignal(doneCh chan error) error {
	fmt.Println("Shutting down process...")
	_ = p.cmd.Process.Signal(syscall.SIGTERM)

	select {
	case err := <-doneCh:
		fmt.Println("Process exited gracefully:", err)
		p.status.Set(ProcessStatusStopped)
		return err

	case <-time.After(2 * time.Second):
		fmt.Println("Process still exiting, sending SIGKILL...")
		_ = p.cmd.Process.Kill()
		err := <-doneCh
		fmt.Println("Process killed:", err)
		p.status.Set(ProcessStatusKilled)

		return err
	}
}

func (p *TaskProcess) Run(config MyTaskConfig) error {
	p.cmd = exec.CommandContext(context.Background(), config.command, config.args...)

	p.cmd.Stdout = os.Stdout
	p.cmd.Stderr = os.Stderr

	fmt.Println("Starting process for", config.name)

	p.cmd.Start()

	doneCh := make(chan error, 1)

	go func() {
		doneCh <- p.cmd.Wait()
	}()

	// Process startup
	select {
	case <-p.stopRequest:
		return p.SendTermSignal(doneCh)

	case err := <-doneCh:
		p.status.Set(ProcessStatusStopped)
		fmt.Println("Process exited early:", err)
		return err

	case <-time.After(2 * time.Second):
		p.status.Set(ProcessStatusRunning)
		fmt.Println("Process has sucessfully started")
	}

	// Process is running
	select {
	case <-p.stopRequest:
		return p.SendTermSignal(doneCh)

	case err := <-doneCh:
		p.status.Set(ProcessStatusStopped)
		fmt.Println("Process exited:", err)
		return err
	}
}
