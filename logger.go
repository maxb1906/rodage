package main

import (
	"io"
	"log"
)

func newLogger(w io.Writer) *log.Logger { return log.New(w, "", log.LstdFlags) }
