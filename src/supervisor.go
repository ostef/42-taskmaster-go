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
	// Represents what the user intended, so we know whether we
	// should spawn new processes when e.g. reloading the config
	shouldRun bool
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

	task.shouldRun = true

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

	task.shouldRun = false

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

	task.shouldRun = true

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

func (s *Supervisor) DestroyTask(name string) error {
	task_idx := slices.IndexFunc(s.tasks, func(t *Task) bool { return t.name == name })
	if task_idx < 0 {
		return fmt.Errorf("No tasked named '%v'", name)
	}

	task := s.tasks[task_idx]

	task.shouldRun = false

	for i, process := range task.processes {
		process.commandQueue <- ProcessCommandDestroy
		task.processes[i] = nil
	}

	task.processes = nil
	s.tasks[task_idx] = nil

	s.tasks = append(s.tasks[:task_idx], s.tasks[task_idx+1:]...)

	return nil
}

func (s *Supervisor) DestroyAllTasks() error {
	for _, task := range s.tasks {
		task.shouldRun = false

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

func (s *Supervisor) UpdateTaskConfig(name string) {
	fmt.Printf("Updating task config for '%v'\n", name)

	config := s.getTaskConfig(name)
	if config == nil {
		fmt.Printf("Task '%v' has been removed from config. Destroying...\n", name)
		s.DestroyTask(name)
	} else {
		task := s.getTask(name)
		if task != nil {
			// Update each running process' config
			for _, process := range task.processes {
				process.setConfig(*config)
			}

			numNewProcessesToSpawn := config.NumProcesses - len(task.processes)
			numProcessesToDestroy := -numNewProcessesToSpawn

			if !task.shouldRun {
				numNewProcessesToSpawn = 0
			}

			if numNewProcessesToSpawn > 0 {
				fmt.Printf("Task '%v' exists and has %v process(es) running. Spawning %v new process(es)\n", name, len(task.processes), numNewProcessesToSpawn)
			} else if numProcessesToDestroy > 0 {
				fmt.Printf("Task '%v' exists and has %v process(es) running. Destroying %v process(es)\n", name, len(task.processes), numProcessesToDestroy)
			}

			// Spawn new processes if necessary
			for range numNewProcessesToSpawn {
				process := new(TaskProcess)
				task.processes = append(task.processes, process)

				process.config = *config
				process.commandQueue = make(chan ProcessCommand, 3)

				s.waitGroup.Go(func() { process.Run(context.Background()) })
			}

			// Destroy processes if necessary
			for range numProcessesToDestroy {
				index := len(task.processes) - 1

				process := task.processes[index]
				process.setConfig(*config)
				process.commandQueue <- ProcessCommandDestroy

				task.processes[index] = nil
				task.processes = task.processes[:index]
			}
		}
	}
}

func (s *Supervisor) ReloadConfig() error {
	fmt.Printf("Reloading config from file '%v'\n", s.config.filename)

	cfg, err := ParseConfig(s.config.filename)
	if err != nil {
		fmt.Println(err)
		return err
	}

	oldcfg := s.config
	s.config = cfg

	// Update tasks that were removed from the config file
	for _, taskConfig := range oldcfg.tasks {
		current := s.getTaskConfig(taskConfig.Name)

		if current == nil {
			s.UpdateTaskConfig(taskConfig.Name)
		}
	}

	for _, taskConfig := range s.config.tasks {
		s.UpdateTaskConfig(taskConfig.Name)
	}

	return nil
}

func (p *TaskProcess) Stop(done chan error) error {
	fmt.Println("Shutting down process...")

	config := p.getConfig()

	_ = p.cmd.Process.Signal(config.StopSignal)
	p.status.Set(ProcessStatusStopping, nil)

	select {
	case err := <-done:
		fmt.Println("Process exited gracefully:", err)
		p.status.Set(ProcessStatusStopped, err)
		return err

	case <-time.After(time.Duration(config.SecondsAfterStopRequestBeforeProgramKill) * time.Second):
		fmt.Println("Process still exiting, sending SIGKILL...")
		_ = p.cmd.Process.Kill()
		err := <-done
		fmt.Println("Process killed:", err)
		p.status.Set(ProcessStatusKilled, err)

		return err
	}
}

func (p *TaskProcess) Run(ctx context.Context) error {
	numAutoRestarts := 0
	for true {
		config := p.getConfig()

		fmt.Printf("Starting process for %v\n", config.Name)

		p.cmd = exec.CommandContext(context.Background(), config.Command, config.Args...)
		p.cmd.Stdout = os.Stdout
		p.cmd.Stderr = os.Stderr

		if config.WorkingDir != "" {
			p.cmd.Dir = config.WorkingDir
		}

		for name, val := range config.Env {
			p.cmd.Env = append(p.cmd.Env, fmt.Sprintf("%v=%v", name, val))
		}

		oldmask := syscall.Umask(int(config.Umask))

		err := p.cmd.Start()
		if err != nil {
			fmt.Println("Could not start process:", err)
			p.status.Set(ProcessStatusError, err)

			return err
		}

		syscall.Umask(oldmask)

		p.status.Set(ProcessStatusStarted, nil)

		doneCh := make(chan error, 1)
		shouldRestart := false

		go func() {
			doneCh <- p.cmd.Wait()
		}()

		// Process startup
		select {
		case err := <-doneCh:
			p.status.Set(ProcessStatusStopped, err)
			fmt.Println("Process exited early:", err)

		case request := <-p.commandQueue:
			switch request {
			case ProcessCommandDestroy:
				return p.Stop(doneCh)

			case ProcessCommandStop:
				p.Stop(doneCh)

			case ProcessCommandRestart:
				p.Stop(doneCh)
				shouldRestart = true
			}

		case <-time.After(time.Duration(config.StartupTimeInSeconds) * time.Second):
			p.status.Set(ProcessStatusRunning, nil)
			fmt.Println("Process has sucessfully started")
		}

		status, _ := p.status.Get()
		if status == ProcessStatusRunning {
			config = p.getConfig()

			select {
			case err := <-doneCh:
				p.status.Set(ProcessStatusStopped, err)
				fmt.Println("Process exited:", err)

			case request := <-p.commandQueue:
				switch request {
				case ProcessCommandDestroy:
					return p.Stop(doneCh)

				case ProcessCommandStop:
					p.Stop(doneCh)

				case ProcessCommandRestart:
					shouldRestart = true
					p.Stop(doneCh)
				}
			}
		}

		config = p.getConfig()

		if config.AutoRestart == AutoRestartAlways && numAutoRestarts < config.MaxAutoRestarts {
			numAutoRestarts += 1
			continue
		}

		numAutoRestarts = 0

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
