//go:build windows

package main

import (
	_ "embed"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"text/template"
	"time"
	"unicode/utf16"
)

//go:embed scheduler_task.xml
var scheduledTaskXMLTemplate string

var taskXMLTemplate = template.Must(template.New("scheduler_task.xml.tmpl").
	Funcs(template.FuncMap{"xmlEscape": escapeXML}).
	Parse(scheduledTaskXMLTemplate))

// installStartupTask creates or updates the configured Task Scheduler entry.
//
// Parameters:
//   - taskName: Scheduler task name.
//   - configPath: Value passed as --config when non-empty.
//   - logPath: Value passed as --log when non-empty.
//   - force: Replaces an existing task with the same name when true.
//
// Returns:
//   - error: Non-nil when XML rendering, temporary file handling, or
//     schtasks execution fails.
func installStartupTask(taskName, configPath, logPath string, force bool) error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("executable: %w", err)
	}

	exePath, err = filepath.Abs(exePath)
	if err != nil {
		return fmt.Errorf("absolute executable path: %w", err)
	}

	if strings.Contains(exePath, "\"") || strings.Contains(configPath, "\"") || strings.Contains(logPath, "\"") {
		return fmt.Errorf("paths must not contain double quotes")
	}

	runArguments := "--background"
	if configPath != "" {
		runArguments += fmt.Sprintf(" --config \"%s\"", configPath)
	}
	if logPath != "" {
		runArguments += fmt.Sprintf(" --log \"%s\"", logPath)
	}
	runArguments = strings.TrimSpace(runArguments)

	username, err := currentUsername()
	if err != nil {
		return err
	}

	xmlContent, err := scheduledTaskXML(username, exePath, runArguments)
	if err != nil {
		return fmt.Errorf("render task xml: %w", err)
	}
	xmlFile, err := os.CreateTemp("", "hotkeys-task-*.xml")
	if err != nil {
		return fmt.Errorf("create temporary task xml: %w", err)
	}
	xmlPath := xmlFile.Name()
	defer os.Remove(xmlPath) //nolint:errcheck

	if err := writeUTF16LEWithBOM(xmlFile, xmlContent); err != nil {
		xmlFile.Close() //nolint:errcheck
		return fmt.Errorf("write task xml: %w", err)
	}
	if err := xmlFile.Close(); err != nil {
		return fmt.Errorf("close task xml: %w", err)
	}

	args := []string{
		"/Create",
		"/TN", taskName,
		"/XML", xmlPath,
	}
	if force {
		args = append(args, "/F")
	}

	if err := runSchTasks(args...); err != nil {
		return fmt.Errorf("create scheduled task: %w", err)
	}

	return nil
}

// scheduledTaskXML renders the embedded XML template used to register the task.
//
// Parameters:
//   - username: Task author and principal user.
//   - command: Executable path used by the Exec action.
//   - arguments: Optional command-line argument string for the Exec action.
//
// Returns:
//   - string: Rendered task XML document.
//   - error: Non-nil when template execution fails.
func scheduledTaskXML(username, command, arguments string) (string, error) {
	startBoundary := time.Now().UTC().Format(time.RFC3339)

	data := struct {
		Author        string
		StartBoundary string
		Command       string
		Arguments     string
	}{
		Author:        username,
		StartBoundary: startBoundary,
		Command:       command,
		Arguments:     arguments,
	}

	var builder strings.Builder
	if err := taskXMLTemplate.Execute(&builder, data); err != nil {
		return "", err
	}

	return builder.String(), nil
}

// escapeXML escapes value for safe inclusion in XML element content.
//
// Parameters:
//   - value: Plain text that may contain XML-sensitive characters.
//
// Returns:
//   - string: Escaped XML text. If escaping fails, value is returned unchanged.
func escapeXML(value string) string {
	var builder strings.Builder
	if err := xml.EscapeText(&builder, []byte(value)); err != nil {
		return value
	}
	return builder.String()
}

// removeStartupTask removes the configured Task Scheduler entry.
//
// Parameters:
//   - taskName: Scheduler task name to delete.
//
// Returns:
//   - error: Non-nil when schtasks cannot delete the task.
func removeStartupTask(taskName string) error {
	if err := runSchTasks("/Delete", "/TN", taskName, "/F"); err != nil {
		return fmt.Errorf("delete scheduled task: %w", err)
	}
	return nil
}

// startStartupTask starts the configured Task Scheduler entry immediately.
//
// Parameters:
//   - taskName: Scheduler task name to run.
//
// Returns:
//   - error: Non-nil when schtasks cannot run the task.
func startStartupTask(taskName string) error {
	if err := runSchTasks("/Run", "/TN", taskName); err != nil {
		return fmt.Errorf("run scheduled task: %w", err)
	}
	return nil
}

// stopProcessGracefully requests graceful shutdown for the running hotkeys process.
//
// Parameters:
//   - pid: Process ID of the hotkeys process.
//
// Returns:
//   - error: Non-nil when graceful stop signaling fails.
func stopProcessGracefully(pid int) error {
	if pid <= 0 {
		return fmt.Errorf("invalid process id: %d", pid)
	}

	if err := signalGracefulStop(); err != nil {
		return fmt.Errorf("signal graceful stop for pid %d: %w", pid, err)
	}

	return nil
}

// scheduledTaskStatus represents whether the configured task exists and its runtime state.
type scheduledTaskStatus struct {
	Scheduled      bool
	Status         string
	ProcessRunning bool
	ProcessID      int
}

// getStartupTaskStatus queries Task Scheduler and returns the current task state.
//
// Parameters:
//   - taskName: Scheduler task name to query.
//
// Returns:
//   - scheduledTaskStatus: Scheduled=false when the task is missing, otherwise
//     populated with the current state.
//   - error: Non-nil when query execution fails for reasons other than a
//     missing task.
func getStartupTaskStatus(taskName string) (scheduledTaskStatus, error) {
	output, err := runSchTasksOutput("/Query", "/TN", taskName, "/FO", "LIST", "/V")
	if err != nil {
		if isTaskNotFoundError(err) {
			return scheduledTaskStatus{Scheduled: false}, nil
		}
		return scheduledTaskStatus{}, fmt.Errorf("query scheduled task: %w", err)
	}

	statusValue := parseTaskStatus(output)

	pid, running, err := findRunningProcessID(IMAGE_NAME, os.Getpid())
	if err != nil {
		return scheduledTaskStatus{}, fmt.Errorf("query running process: %w", err)
	}

	return scheduledTaskStatus{
		Scheduled:      true,
		Status:         statusValue,
		ProcessRunning: running,
		ProcessID:      pid,
	}, nil
}

// currentUsername returns the current Windows username for task principal creation.
//
// Returns:
//   - string: Username from os/user when available, otherwise USERNAME.
//   - error: Non-nil when no username can be resolved.
func currentUsername() (string, error) {
	currentUser, err := user.Current()
	if err == nil && currentUser != nil && currentUser.Username != "" {
		return currentUser.Username, nil
	}

	username := os.Getenv("USERNAME")
	if username == "" {
		return "", fmt.Errorf("determine current username")
	}
	return username, nil
}

// runSchTasks executes schtasks.exe with the provided arguments.
//
// Parameters:
//   - args: Arguments passed directly to schtasks.exe.
//
// Returns:
//   - error: Non-nil when command execution fails.
func runSchTasks(args ...string) error {
	_, err := runSchTasksOutput(args...)
	return err
}

// runSchTasksOutput executes schtasks.exe and returns trimmed combined output.
//
// Parameters:
//   - args: Arguments passed directly to schtasks.exe.
//
// Returns:
//   - string: Trimmed combined output on success.
//   - error: Non-nil on execution failure; includes command output when
//     available.
func runSchTasksOutput(args ...string) (string, error) {
	cmd := exec.Command("schtasks.exe", args...)
	output, err := cmd.CombinedOutput()
	trimmed := strings.TrimSpace(string(output))
	if err != nil {
		if trimmed == "" {
			return "", err
		}
		return "", fmt.Errorf("%w: %s", err, trimmed)
	}
	return trimmed, nil
}

// isTaskNotFoundError reports whether err indicates a missing Scheduler task.
//
// Parameters:
//   - err: Error returned by schtasks execution.
//
// Returns:
//   - bool: True when err text matches known missing-task messages.
func isTaskNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	if strings.Contains(text, "cannot find the file specified") {
		return true
	}
	if strings.Contains(text, "cannot find the task") {
		return true
	}
	return false
}

// parseTaskStatus extracts the "Status" field from schtasks list output.
//
// Parameters:
//   - output: schtasks /Query output in LIST format.
//
// Returns:
//   - string: Parsed status value, or "unknown" when no status field exists.
func parseTaskStatus(output string) string {
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if strings.EqualFold(key, "status") {
			return value
		}
	}
	return "unknown"
}

// writeUTF16LEWithBOM writes content to file as UTF-16LE prefixed with BOM.
//
// Task Scheduler XML import is more reliable when the file encoding matches
// the XML declaration and includes a BOM.
//
// Parameters:
//   - file: Open destination file.
//   - content: UTF-8 source text to encode as UTF-16LE.
//
// Returns:
//   - error: Non-nil when writing the BOM or encoded content fails.
func writeUTF16LEWithBOM(file *os.File, content string) error {
	if _, err := file.Write([]byte{0xFF, 0xFE}); err != nil {
		return err
	}
	encoded := utf16.Encode([]rune(content))
	for _, unit := range encoded {
		if err := binary.Write(file, binary.LittleEndian, unit); err != nil {
			return err
		}
	}
	return nil
}
