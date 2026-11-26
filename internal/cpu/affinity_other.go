//go:build !linux && !windows

package cpu

import "errors"

func pin(n int) error { return errors.New("épinglage non pris en charge") }
