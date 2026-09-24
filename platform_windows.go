//go:build windows

package main

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func openDirectoryInShell(path string) error {
	verb, err := windows.UTF16PtrFromString("open")
	if err != nil {
		return err
	}
	target, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	if err := windows.ShellExecute(0, verb, target, nil, nil, 1); err != nil {
		return fmt.Errorf("open directory: %w", err)
	}
	return nil
}

func openURLInShell(url string) error {
	verb, err := windows.UTF16PtrFromString("open")
	if err != nil {
		return err
	}
	target, err := windows.UTF16PtrFromString(url)
	if err != nil {
		return err
	}
	if err := windows.ShellExecute(0, verb, target, nil, nil, 1); err != nil {
		return fmt.Errorf("open URL: %w", err)
	}
	return nil
}

func showFatalError(message string) {
	text, _ := windows.UTF16PtrFromString(message)
	caption, _ := windows.UTF16PtrFromString("智教教材下载器")
	_, _ = windows.MessageBox(0, text, caption, 0x10)
}
