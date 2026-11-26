//go:build !linux && !windows

package main

import "os/exec"

func openPath(path string) error {
	cmd := exec.Command("open", path)
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}
