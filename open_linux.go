package main

import "os/exec"

// openPath ouvre un fichier ou un dossier local avec l'application par défaut.
// (BrowserOpenURL de Wails refuse les URL file://.)
func openPath(path string) error {
	cmd := exec.Command("xdg-open", path)
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}
