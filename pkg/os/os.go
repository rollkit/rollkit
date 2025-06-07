package os

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
)

type logger interface {
	Info(msg string, keyvals ...any)
}

// TrapSignal catches the SIGTERM/SIGINT and executes cb function. After that it exits
// with code 0.
func TrapSignal(logger logger, cb func()) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		for sig := range c {
			logger.Info("signal trapped", "msg", fmt.Sprintf("captured %v, exiting...", sig))
			if cb != nil {
				cb()
			}
			os.Exit(0)
		}
	}()
}

// EnsureDir ensures the given directory exists, creating it if necessary.
// Errors if the path already exists as a non-directory.
func EnsureDir(dir string, mode os.FileMode) error {
	err := os.MkdirAll(dir, mode)
	if err != nil {
		return fmt.Errorf("could not create directory %q: %w", dir, err)
	}
	return nil
}

// FileExists checks if a file exists.
func FileExists(filePath string) bool {
	_, err := os.Stat(filePath)
	return !os.IsNotExist(err)
}

// ReadFile reads a file and returns the contents.
func ReadFile(filePath string) ([]byte, error) {
	return os.ReadFile(filePath) //nolint:gosec
}

// MustReadFile reads a file and returns the contents. If the file does not exist, it exits the program.
func MustReadFile(filePath string) []byte {
	fileBytes, err := os.ReadFile(filePath) //nolint:gosec
	if err != nil {
		fmt.Println(fmt.Sprintf("MustReadFile failed: %v", err))
		os.Exit(1)
		return nil
	}
	return fileBytes
}

// WriteFile writes a file.
func WriteFile(filePath string, contents []byte, mode os.FileMode) error {
	return os.WriteFile(filePath, contents, mode)
}

// MustWriteFile writes a file. If the file does not exist, it exits the program.
func MustWriteFile(filePath string, contents []byte, mode os.FileMode) {
	err := WriteFile(filePath, contents, mode)
	if err != nil {
		fmt.Println(fmt.Sprintf("MustWriteFile failed: %v", err))
		os.Exit(1)
	}
}

// CopyFile copies a file. It truncates the destination file if it exists.
func CopyFile(src, dst string) error {
	srcfile, err := os.Open(src) //nolint:gosec
	if err != nil {
		return err
	}
	defer func() {
		if cerr := srcfile.Close(); cerr != nil {
			// If we already have an error from the copy operation,
			// prefer that error over the close error
			if err == nil {
				err = cerr
			}
		}
	}()

	info, err := srcfile.Stat()
	if err != nil {
		return err
	}
	if info.IsDir() {
		return errors.New("cannot read from directories")
	}

	// create new file, truncate if exists and apply same permissions as the original one
	dstfile, err := os.OpenFile(dst, os.O_RDWR|os.O_CREATE|os.O_TRUNC, info.Mode().Perm()) //nolint:gosec
	if err != nil {
		return err
	}
	defer func() {
		if cerr := dstfile.Close(); cerr != nil {
			// If we already have an error from the copy operation,
			// prefer that error over the close error
			if err == nil {
				err = cerr
			}
		}
	}()

	_, err = io.Copy(dstfile, srcfile)
	return err
}
