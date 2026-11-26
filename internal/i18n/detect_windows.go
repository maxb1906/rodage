package i18n

import "golang.org/x/sys/windows"

var procGetUserDefaultUILanguage = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetUserDefaultUILanguage")

func detectOS() Lang {
	r, _, _ := procGetUserDefaultUILanguage.Call()
	if r&0x3FF == 0x0C { // LANG_FRENCH
		return FR
	}
	return EN
}
