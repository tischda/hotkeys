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

// installStartupTask creates or updates the configured Task Scheduler entry for hotkeys.
//
// The task launches the current executable with optional config and log arguments.
// When force is true, an existing task with the same name is replaced.
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

	runArguments := ""
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

// escapeXML escapes plain text for safe inclusion in XML element content.
func escapeXML(value string) string {
	var builder strings.Builder
	if err := xml.EscapeText(&builder, []byte(value)); err != nil {
		return value
	}
	return builder.String()
}

// removeStartupTask removes the configured Task Scheduler entry if it exists.
func removeStartupTask(taskName string) error {
	if err := runSchTasks("/Delete", "/TN", taskName, "/F"); err != nil {
		return fmt.Errorf("delete scheduled task: %w", err)
	}
	return nil
}

// scheduledTaskStatus represents whether the configured task exists and its runtime state.
type scheduledTaskStatus struct {
	Scheduled bool
	Running   bool
	Status    string
}

// getStartupTaskStatus queries Task Scheduler and returns the current task state.
func getStartupTaskStatus(taskName string) (scheduledTaskStatus, error) {
	output, err := runSchTasksOutput("/Query", "/TN", taskName, "/FO", "LIST", "/V")
	if err != nil {
		if isTaskNotFoundError(err) {
			return scheduledTaskStatus{Scheduled: false}, nil
		}
		return scheduledTaskStatus{}, fmt.Errorf("query scheduled task: %w", err)
	}

	statusValue := parseTaskStatus(output)
	running := strings.EqualFold(statusValue, "running")

	return scheduledTaskStatus{
		Scheduled: true,
		Running:   running,
		Status:    statusValue,
	}, nil
}

// currentUsername returns the current Windows username for task principal creation.
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

// runSchTasks executes schtasks.exe with the provided arguments and discards output.
func runSchTasks(args ...string) error {
	_, err := runSchTasksOutput(args...)
	return err
}

// runSchTasksOutput executes schtasks.exe and returns trimmed combined output.
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

// isTaskNotFoundError reports whether an schtasks error indicates a missing task.
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

// writeUTF16LEWithBOM writes text to a file as UTF-16LE prefixed with BOM.
//
// Task Scheduler XML import is more reliable when the file encoding matches
// the XML declaration and includes a BOM.
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
