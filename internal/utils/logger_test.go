package utils

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func isolateLogger(t *testing.T) {
	t.Helper()
	t.Chdir(t.TempDir())
	previousLogger := defaultLogger
	previousGlobalLogger := log.Logger
	previousLevel := zerolog.GlobalLevel()
	defaultLogger = nil
	t.Cleanup(func() {
		if defaultLogger != nil {
			if err := defaultLogger.Close(); err != nil {
				t.Error(err)
			}
		}
		defaultLogger = previousLogger
		log.Logger = previousGlobalLogger
		zerolog.SetGlobalLevel(previousLevel)
	})
}

func captureLoggerConsole(t *testing.T, operation func()) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	previousStdout := os.Stdout
	os.Stdout = writer
	defer func() {
		os.Stdout = previousStdout
		_ = reader.Close()
		_ = writer.Close()
	}()
	operation()
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(output)
}

func TestGetLoggerWithUnavailableDefaultFile(t *testing.T) {
	for _, stage := range []string{"directory", "file"} {
		t.Run(stage, func(t *testing.T) {
			isolateLogger(t)
			if stage == "directory" {
				if err := os.WriteFile("logs", []byte("blocking file"), 0600); err != nil {
					t.Fatal(err)
				}
			} else if err := os.MkdirAll(filepath.Join("logs", "wx_channel.log"), 0700); err != nil {
				t.Fatal(err)
			}

			output := captureLoggerConsole(t, func() {
				logger := GetLogger()
				if logger == nil {
					t.Fatal("GetLogger returned nil after a file initialization error")
				}
				if GetLogger() != logger {
					t.Fatal("console logger was not reused")
				}
				Info("console-marker")
				LogConfigLoad("config.yaml", true)
				HandleError(io.ErrUnexpectedEOF, "logger regression")
				LogFileInfo("file-only-marker")
			})
			if !strings.Contains(output, "console-marker") || !strings.Contains(output, "加载配置") {
				t.Fatalf("logging did not remain available: %s", output)
			}
			if !strings.Contains(output, "文件日志不可用") {
				t.Fatalf("file initialization failure was not reported: %s", output)
			}
			if strings.Contains(output, "file-only-marker") {
				t.Fatal("file-only logging leaked to the console")
			}
		})
	}
}

func TestInitLoggerFailureLeavesUsableLogger(t *testing.T) {
	isolateLogger(t)
	if err := os.WriteFile("blocked", []byte("blocking file"), 0600); err != nil {
		t.Fatal(err)
	}
	output := captureLoggerConsole(t, func() {
		if err := InitLoggerWithRotation(INFO, filepath.Join("blocked", "custom.log"), 5); err == nil {
			t.Fatal("expected an initialization error")
		}
		if GetLogger() == nil {
			t.Fatal("failed explicit initialization left a nil logger")
		}
		Info("explicit-failure-marker")
	})
	if !strings.Contains(output, "explicit-failure-marker") {
		t.Fatalf("console logging did not survive explicit initialization failure: %s", output)
	}
	if _, err := os.Stat("logs"); !os.IsNotExist(err) {
		t.Fatalf("explicit failure unexpectedly created the default log directory: %v", err)
	}
}

func TestInitLoggerFailurePreservesExistingFileLogger(t *testing.T) {
	isolateLogger(t)
	if err := InitLoggerWithRotation(INFO, "working.log", 5); err != nil {
		t.Fatal(err)
	}
	previousLogger := GetLogger()
	if err := os.WriteFile("blocked", []byte("blocking file"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := InitLoggerWithRotation(INFO, filepath.Join("blocked", "custom.log"), 5); err == nil {
		t.Fatal("expected an initialization error")
	}
	if GetLogger() != previousLogger {
		t.Fatal("failed reconfiguration replaced a working logger")
	}
	Info("existing-file-marker")
	content, err := os.ReadFile("working.log")
	if err != nil || !strings.Contains(string(content), "existing-file-marker") {
		t.Fatalf("existing file logging did not survive: content=%s, error=%v", content, err)
	}
}

func TestInitLoggerRecoversAfterDefaultFileFailure(t *testing.T) {
	isolateLogger(t)
	if err := os.WriteFile("logs", []byte("blocking file"), 0600); err != nil {
		t.Fatal(err)
	}
	if GetLogger() == nil {
		t.Fatal("expected a console logger")
	}
	if err := InitLoggerWithRotation(INFO, "recovered.log", 5); err != nil {
		t.Fatal(err)
	}
	Info("recovered-file-marker")
	content, err := os.ReadFile("recovered.log")
	if err != nil || !strings.Contains(string(content), "recovered-file-marker") {
		t.Fatalf("file logging did not recover: content=%s, error=%v", content, err)
	}
}

func TestLoggerFileOutputPreservesConsoleBoundary(t *testing.T) {
	isolateLogger(t)
	output := captureLoggerConsole(t, func() {
		Info("public-marker")
		LogFileInfo("private-file-marker")
	})
	if !strings.Contains(output, "public-marker") || strings.Contains(output, "private-file-marker") {
		t.Fatalf("unexpected console output: %s", output)
	}
	content, err := os.ReadFile(filepath.Join("logs", "wx_channel.log"))
	if err != nil || !strings.Contains(string(content), "public-marker") || !strings.Contains(string(content), "private-file-marker") {
		t.Fatalf("unexpected file output: content=%s, error=%v", content, err)
	}
}

func TestConcurrentGetLoggerReturnsSameLogger(t *testing.T) {
	isolateLogger(t)
	const count = 16
	loggers := make([]*Logger, count)
	var group sync.WaitGroup
	for i := range loggers {
		group.Add(1)
		go func() {
			defer group.Done()
			loggers[i] = GetLogger()
		}()
	}
	group.Wait()
	for i, logger := range loggers {
		if logger == nil || logger != loggers[0] {
			t.Fatalf("caller %d received a different or nil logger", i)
		}
	}
}
