package main

import (
	"fmt"
	"os"
	"syscall"

	"gopkg.in/yaml.v3"
)

type AutoRestartPolicy string

const (
	AutoRestartNever      AutoRestartPolicy = "never"
	AutoRestartAlways     AutoRestartPolicy = "always"
	AutoRestartUnexpected AutoRestartPolicy = "unexpected"
)

type RawTask struct {
	Cmd              string            `yaml:"cmd"`
	Args             []string          `yaml:"args"`
	NumProcs         *int              `yaml:"numprocs"`
	Umask            *uint32           `yaml:"umask"`
	WorkingDir       string            `yaml:"workingdir"`
	AutoStart        *bool             `yaml:"autostart"`
	AutoRestart      string            `yaml:"autorestart"`
	ExitCodes        any               `yaml:"exitcodes"`
	AutoRestartTries *int              `yaml:"autorestarttries"`
	AutoStartTime    *float64          `yaml:"autostarttime"`
	StartTime        *float64          `yaml:"starttime"`
	StopSignal       string            `yaml:"stopsignal"`
	StopTime         *float64          `yaml:"stoptime"`
	Stdout           string            `yaml:"stdout"`
	Stderr           string            `yaml:"stderr"`
	Env              map[string]string `yaml:"env"`
}

type RawConfig struct {
	Tasks map[string]RawTask `yaml:"tasks"`
}

type TaskConfig struct {
	Name                                     string
	Command                                  string
	Args                                     []string
	Env                                      map[string]string
	WorkingDir                               string
	NumProcesses                             int
	Stdout                                   string
	Stderr                                   string
	AutoStart                                bool
	SecondsToWaitBeforeAutoStart             float64
	StartupTimeInSeconds                     float64
	AutoRestart                              AutoRestartPolicy
	MaxAutoRestarts                          int
	ExpectedExitCodes                        []int
	StopSignal                               syscall.Signal
	SecondsAfterStopRequestBeforeProcessKill float64
	Umask                                    uint32
}

type Config struct {
	filename string
	tasks    []TaskConfig
}

func setDefaultValueIfNull[T any](currentValue *T, defaultValue T) T {
	if currentValue != nil {
		return *currentValue
	}
	return defaultValue
}

func parseAutoRestart(str string) (AutoRestartPolicy, error) {
	switch str {
	case "", "never":
		return AutoRestartNever, nil
	case "always":
		return AutoRestartAlways, nil
	case "unexpected":
		return AutoRestartUnexpected, nil
	default:
		return "", fmt.Errorf("unknown autorestart policy: %v", str)
	}
}

func parseSignal(str string) (syscall.Signal, error) {
	signals := map[string]syscall.Signal{
		"HUP":  syscall.SIGHUP,
		"INT":  syscall.SIGINT,
		"ILL":  syscall.SIGILL,
		"QUIT": syscall.SIGQUIT,
		"KILL": syscall.SIGKILL,
		"USR1": syscall.SIGUSR1,
		"USR2": syscall.SIGUSR2,
		"TERM": syscall.SIGTERM,
		"STOP": syscall.SIGSTOP,
		"SYS":  syscall.SIGSYS,
		"SEGV": syscall.SIGSEGV,
		"ABRT": syscall.SIGABRT,
		"FPE":  syscall.SIGFPE,
	}

	sig, ok := signals[str]
	if !ok {
		return 0, fmt.Errorf("unknown signal: %v", str)
	}

	return sig, nil
}

func signalToString(sig syscall.Signal) string {
	signals := map[syscall.Signal]string{
		syscall.SIGHUP:  "HUP",
		syscall.SIGINT:  "INT",
		syscall.SIGILL:  "ILL",
		syscall.SIGQUIT: "QUIT",
		syscall.SIGKILL: "KILL",
		syscall.SIGUSR1: "USR1",
		syscall.SIGUSR2: "USR2",
		syscall.SIGTERM: "TERM",
		syscall.SIGSTOP: "STOP",
		syscall.SIGSYS:  "SYS",
		syscall.SIGSEGV: "SEGV",
		syscall.SIGABRT: "ABRT",
		syscall.SIGFPE:  "FPE",
	}

	str, ok := signals[sig]
	if !ok {
		return "???"
	}

	return str
}

func parseExitCodes(exitCodes any) ([]int, error) {
	switch value := exitCodes.(type) {
	case nil:
		return []int{0}, nil

	case int:
		return []int{value}, nil

	case []any:
		codes := make([]int, 0, len(value))
		for _, code := range value {
			number, ok := code.(int)
			if !ok {
				return nil, fmt.Errorf("invalid exit code: %v", code)
			}

			codes = append(codes, number)
		}

		return codes, nil

	default:
		return nil, fmt.Errorf("invalid exit code")
	}
}

func validateTask(task TaskConfig, taskName string) error {
	if task.NumProcesses < 1 {
		return fmt.Errorf("task %v: numprocs must be >= 1, got %v", taskName, task.NumProcesses)
	}
	if task.SecondsToWaitBeforeAutoStart < 0 {
		return fmt.Errorf("task %v: starttime must be >= 0, got %v", taskName, task.SecondsToWaitBeforeAutoStart)
	}
	if task.MaxAutoRestarts <= 0 {
		return fmt.Errorf("task %v: autorestarttries must be > 0, got %v", taskName, task.MaxAutoRestarts)
	}
	if task.SecondsAfterStopRequestBeforeProcessKill < 0 {
		return fmt.Errorf("task %v: stoptime must be >= 0, got %v", taskName, task.SecondsAfterStopRequestBeforeProcessKill)
	}
	if task.Umask > 0777 {
		return fmt.Errorf("task %v: umask must be <= 0777, got 0%o", taskName, task.Umask)
	}

	return nil
}

func ParseConfig(file_path string) (Config, error) {
	data, err := os.ReadFile(file_path)
	if err != nil {
		return Config{}, fmt.Errorf("cannot read file %v: %w", file_path, err)
	}
	var raw RawConfig
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return Config{}, fmt.Errorf("invalid YAML from %v: %w", file_path, err)
	}

	defaultUmask := syscall.Umask(0)
	syscall.Umask(defaultUmask)

	config := Config{filename: file_path}

	for name, tasks := range raw.Tasks {
		if tasks.Cmd == "" {
			return Config{}, fmt.Errorf("task %v: cmd is required", name)
		}

		autoRestart, err := parseAutoRestart(tasks.AutoRestart)
		if err != nil {
			return Config{}, fmt.Errorf("task %v, autorestart error:  %w", name, err)
		}

		stopSignal := syscall.SIGTERM
		if tasks.StopSignal != "" {
			stopSignal, err = parseSignal(tasks.StopSignal)
			if err != nil {
				return Config{}, fmt.Errorf("task %v, stopsignal error: %w", name, err)
			}
		}

		exitCodes, err := parseExitCodes(tasks.ExitCodes)
		if err != nil {
			return Config{}, fmt.Errorf("task %v, exit code error: %w", name, err)
		}

		task := TaskConfig{
			Name:                                     name,
			Command:                                  tasks.Cmd,
			Args:                                     tasks.Args,
			Env:                                      tasks.Env,
			WorkingDir:                               tasks.WorkingDir,
			NumProcesses:                             setDefaultValueIfNull(tasks.NumProcs, 1),
			Stdout:                                   tasks.Stdout,
			Stderr:                                   tasks.Stderr,
			AutoStart:                                setDefaultValueIfNull(tasks.AutoStart, false),
			SecondsToWaitBeforeAutoStart:             setDefaultValueIfNull(tasks.AutoStartTime, 0),
			StartupTimeInSeconds:                     setDefaultValueIfNull(tasks.StartTime, 1),
			AutoRestart:                              autoRestart,
			MaxAutoRestarts:                          setDefaultValueIfNull(tasks.AutoRestartTries, 5),
			ExpectedExitCodes:                        exitCodes,
			StopSignal:                               stopSignal,
			Umask:                                    setDefaultValueIfNull(tasks.Umask, uint32(defaultUmask)),
			SecondsAfterStopRequestBeforeProcessKill: setDefaultValueIfNull(tasks.StopTime, 5),
		}

		err = validateTask(task, name)
		if err != nil {
			return Config{}, err
		}

		config.tasks = append(config.tasks, task)
	}

	return config, nil
}
