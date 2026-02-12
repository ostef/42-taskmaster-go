package main

import (
	"fmt"
	"os"
	"path/filepath"
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
	Cmd          string            `yaml:"cmd"`
	NumProcs     *int              `yaml:"numprocs"`
	Umask        *uint32           `yaml:"umask"`
	WorkingDir   string            `yaml:"workingdir"`
	AutoStart    *bool             `yaml:"autostart"`
	AutoRestart  string            `yaml:"autorestart"`
	ExitCodes    any               `yaml:"exitcodes"`
	StartRetries *int              `yaml:"startretries"`
	StartTime    *float64          `yaml:"starttime"`
	StopSignal   string            `yaml:"stopsignal"`
	StopTime     *float64          `yaml:"stoptime"`
	Stdout       string            `yaml:"stdout"`
	Stderr       string            `yaml:"stderr"`
	Env          map[string]string `yaml:"env"`
}

type RawConfig struct {
	Programs map[string]RawTask `yaml:"programs"`
}

type TaskConfig struct {
	name                                           string
	command_line                                   string
	env                                            map[string]string
	working_directory                              string
	num_processes                                  int
	stdout_log_file                                string
	stderr_log_file                                string
	auto_start                                     bool
	seconds_to_wait_before_auto_start              float64
	auto_restart                                   AutoRestartPolicy
	max_auto_restarts                              int
	expected_exit_codes                            []int
	stop_signal                                    syscall.Signal
	seconds_after_stop_request_before_program_kill float64
	umask                                          uint32
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
		return "", fmt.Errorf("unknown autorestart policy: %s", str)
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
		return 0, fmt.Errorf("unknown signal: %s", str)
	}
	return sig, nil
}

func checkExitCode(code int) error {
	if code < 0 || code > 255 {
		return fmt.Errorf("exit code out of range [0-255]: %d", code)
	}
	return nil
}

func parseExitCodes(exitCodes any) ([]int, error) {
	switch value := exitCodes.(type) {
	case nil:
		return []int{0}, nil
	case int:
		err := checkExitCode(value)
		if err != nil {
			return nil, err
		}
		return []int{value}, nil
	case []any:
		codes := make([]int, 0, len(value))
		for _, code := range value {
			number, ok := code.(int)
			if !ok {
				return nil, fmt.Errorf("invalid exit code: %v", code)
			}
			err := checkExitCode(number)
			if err != nil {
				return nil, err
			}
			codes = append(codes, number)
		}
		return codes, nil
	default:
		return nil, fmt.Errorf("invalid exit code")
	}
}

func validateTask(task TaskConfig, programName string) error {
	if task.num_processes < 1 {
		return fmt.Errorf("program %s: numprocs must be >= 1, got %d", programName, task.num_processes)
	}
	if task.seconds_to_wait_before_auto_start < 0 {
		return fmt.Errorf("program %s: starttime must be >= 0, got %v", programName, task.seconds_to_wait_before_auto_start)
	}
	if task.max_auto_restarts < 0 {
		return fmt.Errorf("program %s: startretries must be >= 0, got %d", programName, task.max_auto_restarts)
	}
	if task.seconds_after_stop_request_before_program_kill < 0 {
		return fmt.Errorf("program %s: stoptime must be >= 0, got %v", programName, task.seconds_after_stop_request_before_program_kill)
	}
	if task.umask > 0777 {
		return fmt.Errorf("program %s: umask must be <= 0777, got %o", programName, task.umask)
	}

	return nil
}

func ParseConfig(file_path string) (*Config, error) {
	data, err := os.ReadFile(file_path)
	if err != nil {
		return nil, fmt.Errorf("cannot read file %s: %w", file_path, err)
	}
	var raw RawConfig
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid YAML from %s: %w", file_path, err)
	}

	filename := filepath.Base(file_path)
	config := Config{filename: filename}

	for name, tasks := range raw.Programs {
		if tasks.Cmd == "" {
			return nil, fmt.Errorf("program %s: cmd is required", name)
		}

		autoRestart, err := parseAutoRestart(tasks.AutoRestart)
		if err != nil {
			return nil, fmt.Errorf("program %s, autorestart error:  %w", name, err)
		}
		stopSignal := syscall.SIGTERM
		if tasks.StopSignal != "" {
			stopSignal, err = parseSignal(tasks.StopSignal)
			if err != nil {
				return nil, fmt.Errorf("program %s, stopsignal error: %w", name, err)
			}
		}

		exitCodes, err := parseExitCodes(tasks.ExitCodes)
		if err != nil {
			return nil, fmt.Errorf("program %s, exit code error: %w", name, err)
		}

		task := TaskConfig{
			name:                              name,
			command_line:                      tasks.Cmd,
			env:                               tasks.Env,
			working_directory:                 tasks.WorkingDir,
			num_processes:                     setDefaultValueIfNull(tasks.NumProcs, 1),
			stdout_log_file:                   tasks.Stdout,
			stderr_log_file:                   tasks.Stderr,
			auto_start:                        setDefaultValueIfNull(tasks.AutoStart, false),
			seconds_to_wait_before_auto_start: setDefaultValueIfNull(tasks.StartTime, 0),
			auto_restart:                      autoRestart,
			max_auto_restarts:                 setDefaultValueIfNull(tasks.StartRetries, 5),
			expected_exit_codes:               exitCodes,
			stop_signal:                       stopSignal,
			umask:                             setDefaultValueIfNull(tasks.Umask, uint32(0)),
			seconds_after_stop_request_before_program_kill: setDefaultValueIfNull(tasks.StopTime, 5),
		}

		err = validateTask(task, name)
		if err != nil {
			return nil, err
		}
		config.tasks = append(config.tasks, task)
	}
	return &config, nil
}
