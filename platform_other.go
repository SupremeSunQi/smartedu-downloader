//go:build !windows

package main

import (
	"fmt"
	"os/exec"
)

func openDirectoryInShell(path string) error {
	return exec.Command("xdg-open", path).Start()
}

func openURLInShell(url string) error {
	return exec.Command("xdg-open", url).Start()
}

func showFatalError(message string) {
	fmt.Println(message)
}
