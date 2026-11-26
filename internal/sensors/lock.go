package sensors

import "runtime"

func runtimeLock()   { runtime.LockOSThread() }
func runtimeUnlock() { runtime.UnlockOSThread() }
