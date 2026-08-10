package docker

import (
	"context"
	"io"

	"github.com/moby/moby/client"

	"dockman/internal/model"
)

type terminalSession struct {
	client *client.Client
	execID string
	attach client.HijackedResponse
}

func (m *Manager) OpenContainerTerminal(ctx context.Context, containerID string, width, height uint) (model.TerminalSession, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.client == nil {
		return nil, ErrNotConnected
	}
	command := []string{"/bin/sh", "-c", "if [ -x /bin/bash ]; then exec /bin/bash; else exec /bin/sh; fi"}
	created, err := m.client.ExecCreate(ctx, containerID, client.ExecCreateOptions{TTY: true, AttachStdin: true, AttachStdout: true, AttachStderr: true, Cmd: command, ConsoleSize: client.ConsoleSize{Height: height, Width: width}})
	if err != nil {
		return nil, err
	}
	attached, err := m.client.ExecAttach(ctx, created.ID, client.ExecAttachOptions{TTY: true, ConsoleSize: client.ConsoleSize{Height: height, Width: width}})
	if err != nil {
		return nil, err
	}
	return &terminalSession{client: m.client, execID: created.ID, attach: attached.HijackedResponse}, nil
}

func (s *terminalSession) Read(p []byte) (int, error)  { return s.attach.Reader.Read(p) }
func (s *terminalSession) Write(p []byte) (int, error) { return s.attach.Conn.Write(p) }
func (s *terminalSession) Close() error                { s.attach.Close(); return nil }
func (s *terminalSession) Resize(ctx context.Context, width, height uint) error {
	_, err := s.client.ExecResize(ctx, s.execID, client.ExecResizeOptions{Width: width, Height: height})
	return err
}

var _ io.ReadWriteCloser = (*terminalSession)(nil)
