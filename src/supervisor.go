package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"slices"
	"sync"
	"syscall"
	"time"
)

type ProcessStatusNoMutex struct {
	value        uint
	byUser       bool
	expectedExit bool
	exitCode     int
	err          error
}

type ProcessStatus struct {
	ProcessStatusNoMutex
	mutex sync.RWMutex
}

func (s *ProcessStatus) SetExited(status uint, byUser bool, err error, expectedExitCodes []int) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.exitCode = 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			s.exitCode = exitErr.ExitCode()
		}
	}

	s.expectedExit = slices.Contains(expectedExitCodes, s.exitCode)
	s.value = status
	s.byUser = byUser
}

func (s *ProcessStatus) Set(status uint, err error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.exitCode = 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			s.exitCode = exitErr.ExitCode()
		}
	}

	s.value = status
	s.err = err
	s.expectedExit = false
	s.byUser = false
}

func (s *ProcessStatus) Get() (uint, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	return s.value, s.err
}

func (s *ProcessStatus) GetAll() ProcessStatusNoMutex {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	copied := s.ProcessStatusNoMutex
	return copied
}

func (s *ProcessStatus) String() string {
	status := s.GetAll()

	switch status.value {
	case ProcessStatusNotStarted:
		return "not started"
	case ProcessStatusStarted:
		return "started"
	case ProcessStatusRunning:
		return "running"
	case ProcessStatusStopping:
		return "stopping"
	case ProcessStatusStopped:
		if status.expectedExit {
			if status.byUser {
				return fmt.Sprintf("stopped by user (expected, %v)", status.exitCode)
			} else {
				return fmt.Sprintf("stopped (expected, %v)", status.exitCode)
			}
		} else {
			if status.byUser {
				return fmt.Sprintf("stopped by user (unexpected, %v)", status.exitCode)
			} else {
				return fmt.Sprintf("stopped (unexpected, %v)", status.exitCode)
			}
		}

	case ProcessStatusKilled:
		return fmt.Sprintf("killed (%v)", status.err)

	case ProcessStatusError:
		return fmt.Sprintf("error (%v)", status.err)
	default:
		return fmt.Sprintf("??? (%v)", status.err)
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
	logger       *log.Logger
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
	waitGroup    sync.WaitGroup
	tasks        []*Task
	config       Config
	isRunning    bool
	commandQueue chan SupervisorCommand
	sigChan      chan os.Signal
	logger       *log.Logger
	logFile      *os.File
}

const (
	SupervisorStartTask    = iota
	SupervisorStopTask     = iota
	SupervisorRestartTask  = iota
	SupervisorExit         = iota
	SupervisorPrintStatus  = iota
	SupervisorReloadConfig = iota
)

type SupervisorCommand struct {
	Kind     uint
	TaskName string
	ErrChan  chan error
}

func (s *Supervisor) Init(config Config, f *os.File) {
	s.commandQueue = make(chan SupervisorCommand, 1)
	s.sigChan = make(chan os.Signal, 1)

	s.config = config
	signal.Notify(s.sigChan, syscall.SIGHUP)

	s.isRunning = true

	s.logFile = f
	s.logger = log.New(f, "[taskmaster] ", log.LstdFlags)
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

func (s *Supervisor) loggerErrorf(format string, args ...any) error {
	s.logger.Printf(format, args...)
	return fmt.Errorf(format, args...)
}

func (s *Supervisor) StartTask(name string) error {
	config := s.getTaskConfig(name)
	if config == nil {
		return s.loggerErrorf("No task named '%v'", name)
	}
	s.logger.Printf("Starting task '%s'", name)
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
			return s.loggerErrorf("Task '%v' still has some running processes", name)
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

		process.setConfig(*config)
		process.commandQueue = make(chan ProcessCommand, 3)
		process.logger = s.logger

		s.waitGroup.Go(func() { process.Run(context.Background()) })
	}

	return nil
}

func (s *Supervisor) StopTask(name string) error {
	task := s.getTask(name)
	if task == nil {
		return s.loggerErrorf("No task named '%v'", name)
	}

	s.logger.Printf("Stopping task '%s'", name)
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
		return s.loggerErrorf("No task named '%v'", name)
	}

	s.logger.Printf("Restarting task '%s'", name)
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

	numNewProcessesToSpawn := config.NumProcesses - len(task.processes)
	for range numNewProcessesToSpawn {
		process := new(TaskProcess)
		task.processes = append(task.processes, process)

		process.setConfig(*config)
		process.commandQueue = make(chan ProcessCommand, 3)
		process.logger = s.logger

		s.waitGroup.Go(func() { process.Run(context.Background()) })
	}

	return nil
}

func (s *Supervisor) DestroyTask(name string) error {
	task_idx := slices.IndexFunc(s.tasks, func(t *Task) bool { return t.name == name })
	if task_idx < 0 {
		return s.loggerErrorf("No tasked named '%v'", name)
	}

	task := s.tasks[task_idx]

	s.logger.Printf("Destroying task '%s'", name)
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
	s.logger.Printf("Destroying all tasks")
	for _, task := range s.tasks {
		task.shouldRun = false

		for _, process := range task.processes {
			process.commandQueue <- ProcessCommandDestroy
		}
	}

	s.waitGroup.Wait()

	s.tasks = nil
	s.logFile.Close()

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
	s.logger.Printf("Updating task config for '%s'", name)
	config := s.getTaskConfig(name)
	if config == nil {
		s.logger.Printf("Task '%s' removed from config, destroying", name)
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
				s.logger.Printf("Task '%v' exists and has %v process(es) running. Spawning %v new process(es)\n", name, len(task.processes), numNewProcessesToSpawn)
			} else if numProcessesToDestroy > 0 {
				s.logger.Printf("Task '%v' exists and has %v process(es) running. Destroying %v process(es)\n", name, len(task.processes), numProcessesToDestroy)
			}

			// Spawn new processes if necessary
			for range numNewProcessesToSpawn {
				process := new(TaskProcess)
				task.processes = append(task.processes, process)

				process.setConfig(*config)
				process.commandQueue = make(chan ProcessCommand, 3)
				process.logger = s.logger

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
	s.logger.Printf("Reloading config from '%s'", s.config.filename)
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

	s.logger.Printf("Config reloaded successfully")

	return nil
}

func (s *Supervisor) Loop() {
	defer s.logger.Println("Exiting")

	for s.isRunning {
		select {
		case cmd := <-s.commandQueue:
			switch cmd.Kind {
			case SupervisorStartTask:
				err := s.StartTask(cmd.TaskName)
				if cmd.ErrChan != nil {
					cmd.ErrChan <- err
				}
			case SupervisorStopTask:
				err := s.StopTask(cmd.TaskName)
				if cmd.ErrChan != nil {
					cmd.ErrChan <- err
				}
			case SupervisorRestartTask:
				err := s.RestartTask(cmd.TaskName)
				if cmd.ErrChan != nil {
					cmd.ErrChan <- err
				}
			case SupervisorExit:
				err := s.DestroyAllTasks()
				s.isRunning = false
				if cmd.ErrChan != nil {
					cmd.ErrChan <- err
				}
			case SupervisorPrintStatus:
				s.PrintStatus()
				if cmd.ErrChan != nil {
					cmd.ErrChan <- nil
				}
			case SupervisorReloadConfig:
				err := s.ReloadConfig()
				if cmd.ErrChan != nil {
					cmd.ErrChan <- err
				}
			}

		case sig := <-s.sigChan:
			if sig == syscall.SIGHUP {
				s.logger.Println("Received SIGHUP, reloading config")
				_ = s.ReloadConfig()
			}
		}
	}
}

func (p *TaskProcess) Stop(done chan error) error {
	p.logger.Println("Shutting down process...")

	config := p.getConfig()

	_ = p.cmd.Process.Signal(config.StopSignal)
	p.status.Set(ProcessStatusStopping, nil)

	select {
	case err := <-done:
		p.logger.Println("Process exited gracefully:", err)
		p.status.SetExited(ProcessStatusStopped, true, err, config.ExpectedExitCodes)
		return err

	case <-time.After(time.Duration(config.SecondsAfterStopRequestBeforeProgramKill) * time.Second):
		p.logger.Println("Process still exiting, sending SIGKILL...")
		_ = p.cmd.Process.Kill()
		err := <-done
		p.logger.Println("Process killed:", err)
		p.status.Set(ProcessStatusKilled, err)

		return err
	}
}

func (p *TaskProcess) Run(ctx context.Context) error {
	numAutoRestarts := 0
	for true {
		config := p.getConfig()

		p.logger.Printf("Starting process for '%s'", config.Name)
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
			p.logger.Println("Could not start process:", err)
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
			p.status.SetExited(ProcessStatusStopped, false, err, config.ExpectedExitCodes)
			p.logger.Println("Process exited early:", err)

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
			p.logger.Println("Process has sucessfully started")
		}

		status, _ := p.status.Get()
		if status == ProcessStatusRunning {
			config = p.getConfig()

			select {
			case err := <-doneCh:
				p.status.SetExited(ProcessStatusStopped, false, err, config.ExpectedExitCodes)
				p.logger.Println("Process exited:", err)

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

		allStatus := p.status.GetAll()
		stoppedByUser := allStatus.value == ProcessStatusStopped && !allStatus.byUser

		if stoppedByUser && (config.AutoRestart == AutoRestartAlways || (config.AutoRestart == AutoRestartUnexpected && !allStatus.expectedExit)) && numAutoRestarts < config.MaxAutoRestarts {
			numAutoRestarts += 1
			p.logger.Printf("Auto-restarting '%s' (attempt %d/%d)", config.Name, numAutoRestarts, config.MaxAutoRestarts)
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
