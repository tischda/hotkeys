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

const defaultTaskName = "Hotkeys"

//go:embed scheduler_task.xml
var scheduledTaskXMLTemplate string

var taskXMLTemplate = template.Must(template.New("scheduler_task.xml.tmpl").
	Funcs(template.FuncMap{"xmlEscape": escapeXML}).
	Parse(scheduledTaskXMLTemplate))

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

func escapeXML(value string) string {
	var builder strings.Builder
	if err := xml.EscapeText(&builder, []byte(value)); err != nil {
		return value
	}
	return builder.String()
}

func removeStartupTask(taskName string) error {
	if err := runSchTasks("/Delete", "/TN", taskName, "/F"); err != nil {
		return fmt.Errorf("delete scheduled task: %w", err)
	}
	return nil
}

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

func runSchTasks(args ...string) error {
	cmd := exec.Command("schtasks.exe", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(output))
		if trimmed == "" {
			return err
		}
		return fmt.Errorf("%w: %s", err, trimmed)
	}
	return nil
}

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
