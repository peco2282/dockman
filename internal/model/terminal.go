package model

import (
	"context"
	"io"
)

type TerminalSession interface {
	io.ReadWriteCloser
	Resize(context.Context, uint, uint) error
}

type TerminalEvent struct {
	Data []byte
	Err  error
	Done bool
}
