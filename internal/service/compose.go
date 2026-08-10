package service

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"dockman/internal/model"
)

type ComposeRepository interface {
	ListComposeProjects(context.Context) ([]model.ComposeProject, error)
}
type ComposeCommandRunner interface {
	Run(context.Context, model.ComposeProject, ...string) error
	Stream(context.Context, model.ComposeProject, ...string) (io.ReadCloser, error)
}
type ComposeAction int

const (
	ComposeUp ComposeAction = iota
	ComposeDown
	ComposeRestart
	ComposePull
	ComposeBuild
	ComposeUpBuild
	ComposeStop
	ComposeStart
	ComposePause
	ComposeDownRemoveOrphans
)

func (a ComposeAction) String() string {
	switch a {
	case ComposeUp:
		return "Up"
	case ComposeDown:
		return "Down"
	case ComposeRestart:
		return "Restart"
	case ComposePull:
		return "Pull"
	case ComposeBuild:
		return "Build"
	case ComposeUpBuild:
		return "Up with build"
	case ComposeStop:
		return "Stop"
	case ComposeStart:
		return "Start"
	case ComposePause:
		return "Pause"
	case ComposeDownRemoveOrphans:
		return "Down + remove orphans"
	default:
		return "Unknown"
	}
}

type ComposeService struct {
	repository   ComposeRepository
	runner       ComposeCommandRunner
	timeout      time.Duration
	mu           sync.Mutex
	known        map[string]model.ComposeProject
	registryPath string
	registryErr  error
}

func NewComposeService(repository ComposeRepository, runner ComposeCommandRunner) *ComposeService {
	path, err := defaultComposeRegistryPath()
	service := NewComposeServiceWithRegistry(repository, runner, path)
	if err != nil {
		service.registryErr = err
	}
	return service
}
func NewComposeServiceWithRegistry(repository ComposeRepository, runner ComposeCommandRunner, registryPath string) *ComposeService {
	s := &ComposeService{repository: repository, runner: runner, timeout: 15 * time.Second, known: map[string]model.ComposeProject{}, registryPath: registryPath}
	s.registryErr = s.loadRegisteredProjects()
	return s
}
func (s *ComposeService) List(ctx context.Context) ([]model.ComposeProject, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	current, err := s.repository.ListComposeProjects(ctx)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.registryErr != nil {
		return nil, s.registryErr
	}
	seen := map[string]bool{}
	for _, project := range current {
		if registered, ok := s.known[project.Name]; ok && registered.Registered {
			project.Registered = true
			if project.WorkingDir == "" {
				project.WorkingDir = registered.WorkingDir
			}
			if len(project.ConfigFiles) == 0 {
				project.ConfigFiles = registered.ConfigFiles
			}
		}
		s.known[project.Name] = project
		seen[project.Name] = true
	}
	projects := make([]model.ComposeProject, 0, len(s.known))
	for name, project := range s.known {
		if !seen[name] {
			project.Containers = nil
			s.known[name] = project
		}
		projects = append(projects, project)
	}
	sort.Slice(projects, func(i, j int) bool { return projects[i].Name < projects[j].Name })
	return projects, nil
}
func (s *ComposeService) Execute(ctx context.Context, project model.ComposeProject, action ComposeAction) error {
	if err := validateComposeProject(project); err != nil {
		return err
	}
	timeout := 2 * time.Minute
	if action == ComposePull {
		timeout = 10 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	switch action {
	case ComposeUp:
		return s.runner.Run(ctx, project, "up", "--detach")
	case ComposeDown:
		return s.runner.Run(ctx, project, "down")
	case ComposeRestart:
		return s.runner.Run(ctx, project, "restart")
	case ComposePull:
		return s.runner.Run(ctx, project, "pull")
	case ComposeBuild:
		return s.runner.Run(ctx, project, "build")
	case ComposeUpBuild:
		return s.runner.Run(ctx, project, "up", "--detach", "--build")
	case ComposeStop:
		return s.runner.Run(ctx, project, "stop")
	case ComposeStart:
		return s.runner.Run(ctx, project, "start")
	case ComposePause:
		return s.runner.Run(ctx, project, "pause")
	case ComposeDownRemoveOrphans:
		return s.runner.Run(ctx, project, "down", "--remove-orphans")
	default:
		return errors.New("unsupported Compose action")
	}
}

// Unregister removes a saved project from dockman's registry. It never runs
// docker compose down and therefore does not alter containers or files.
func (s *ComposeService) Unregister(_ context.Context, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	project, ok := s.known[name]
	if !ok || !project.Registered {
		return errors.New("Compose project is not registered")
	}
	if len(project.Containers) > 0 {
		project.Registered = false
		s.known[name] = project
	} else {
		delete(s.known, name)
	}
	if err := s.saveRegisteredProjectsLocked(); err != nil {
		s.known[name] = project
		return err
	}
	return nil
}

type ComposeRegisterInput struct {
	Name        string
	WorkingDir  string
	ConfigFiles string
}

var composeProjectNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

func (s *ComposeService) Register(_ context.Context, input ComposeRegisterInput) (model.ComposeProject, error) {
	name := strings.ToLower(strings.TrimSpace(input.Name))
	if !composeProjectNamePattern.MatchString(name) {
		return model.ComposeProject{}, errors.New("project name must use lowercase letters, numbers, dashes, or underscores")
	}
	workingDir, err := filepath.Abs(strings.TrimSpace(input.WorkingDir))
	if err != nil || strings.TrimSpace(input.WorkingDir) == "" {
		return model.ComposeProject{}, errors.New("enter a valid Compose working directory")
	}
	stat, err := os.Stat(workingDir)
	if err != nil || !stat.IsDir() {
		return model.ComposeProject{}, errors.New("Compose working directory does not exist or is not a directory")
	}
	files := splitCommaList(input.ConfigFiles)
	if len(files) == 0 {
		for _, candidate := range []string{"compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml"} {
			if info, statErr := os.Stat(filepath.Join(workingDir, candidate)); statErr == nil && !info.IsDir() {
				files = []string{candidate}
				break
			}
		}
	}
	if len(files) == 0 {
		return model.ComposeProject{}, errors.New("no Compose file found; enter a config file such as compose.yaml")
	}
	resolved := make([]string, 0, len(files))
	for _, file := range files {
		if !filepath.IsAbs(file) {
			file = filepath.Join(workingDir, file)
		}
		file = filepath.Clean(file)
		info, statErr := os.Stat(file)
		if statErr != nil || info.IsDir() {
			return model.ComposeProject{}, errors.New("Compose config file does not exist: " + file)
		}
		resolved = append(resolved, file)
	}
	project := model.ComposeProject{Name: name, WorkingDir: workingDir, ConfigFiles: resolved, Registered: true}
	s.mu.Lock()
	defer s.mu.Unlock()
	previous, hadPrevious := s.known[name]
	if hadPrevious {
		project.Containers = previous.Containers
	}
	s.known[name] = project
	if err := s.saveRegisteredProjectsLocked(); err != nil {
		if hadPrevious {
			s.known[name] = previous
		} else {
			delete(s.known, name)
		}
		return model.ComposeProject{}, err
	}
	return project, nil
}

func defaultComposeRegistryPath() (string, error) {
	directory, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, "dockman", "compose-projects.json"), nil
}
func (s *ComposeService) loadRegisteredProjects() error {
	if s.registryPath == "" {
		return nil
	}
	data, err := os.ReadFile(s.registryPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return errors.New("could not read saved Compose projects: " + err.Error())
	}
	var projects []model.ComposeProject
	if err := json.Unmarshal(data, &projects); err != nil {
		return errors.New("saved Compose project list is invalid: " + err.Error())
	}
	for _, project := range projects {
		project.Registered = true
		project.Containers = nil
		s.known[project.Name] = project
	}
	return nil
}
func (s *ComposeService) saveRegisteredProjectsLocked() error {
	if s.registryPath == "" {
		return nil
	}
	projects := make([]model.ComposeProject, 0)
	for _, project := range s.known {
		if project.Registered {
			project.Containers = nil
			projects = append(projects, project)
		}
	}
	sort.Slice(projects, func(i, j int) bool { return projects[i].Name < projects[j].Name })
	data, err := json.MarshalIndent(projects, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.registryPath), 0700); err != nil {
		return errors.New("could not create dockman config directory: " + err.Error())
	}
	if err := os.WriteFile(s.registryPath, data, 0600); err != nil {
		return errors.New("could not save Compose project: " + err.Error())
	}
	return nil
}
func (s *ComposeService) FollowLogs(ctx context.Context, project model.ComposeProject) (<-chan LogEvent, error) {
	if err := validateComposeProject(project); err != nil {
		return nil, err
	}
	reader, err := s.runner.Stream(ctx, project, "logs", "--follow", "--tail", "500", "--no-color")
	if err != nil {
		return nil, err
	}
	events := make(chan LogEvent, 128)
	go func() {
		defer close(events)
		defer reader.Close()
		scanner := bufio.NewScanner(reader)
		buffer := make([]byte, 64*1024)
		scanner.Buffer(buffer, 1024*1024)
		for scanner.Scan() {
			select {
			case events <- LogEvent{Line: scanner.Text()}:
			case <-ctx.Done():
				return
			}
		}
		if err := scanner.Err(); err != nil && ctx.Err() == nil {
			select {
			case events <- LogEvent{Err: err, Done: true}:
			case <-ctx.Done():
			}
			return
		}
		select {
		case events <- LogEvent{Done: true}:
		case <-ctx.Done():
		}
	}()
	return events, nil
}
func validateComposeProject(project model.ComposeProject) error {
	if strings.TrimSpace(project.Name) == "" {
		return errors.New("Compose project name is missing")
	}
	if project.WorkingDir == "" && len(project.ConfigFiles) == 0 {
		return errors.New("Compose configuration location is unavailable for this project")
	}
	return nil
}

type contextNameProvider interface{ ContextName() string }
type DockerComposeRunner struct {
	binary   string
	contexts contextNameProvider
}

func NewDockerComposeRunner(contexts ...contextNameProvider) *DockerComposeRunner {
	r := &DockerComposeRunner{binary: "docker"}
	if len(contexts) > 0 {
		r.contexts = contexts[0]
	}
	return r
}
func (r *DockerComposeRunner) Run(ctx context.Context, project model.ComposeProject, operation ...string) error {
	command := exec.CommandContext(ctx, r.binary, r.composeArgs(project, operation...)...)
	command.Dir = project.WorkingDir
	output, err := command.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message != "" {
			return errors.New(message)
		}
		return err
	}
	return nil
}
func (r *DockerComposeRunner) Stream(ctx context.Context, project model.ComposeProject, operation ...string) (io.ReadCloser, error) {
	reader, writer := io.Pipe()
	command := exec.CommandContext(ctx, r.binary, r.composeArgs(project, operation...)...)
	command.Dir = project.WorkingDir
	command.Stdout = writer
	command.Stderr = writer
	if err := command.Start(); err != nil {
		_ = reader.Close()
		_ = writer.Close()
		return nil, err
	}
	go func() { err := command.Wait(); _ = writer.CloseWithError(err) }()
	return reader, nil
}
func (r *DockerComposeRunner) composeArgs(project model.ComposeProject, operation ...string) []string {
	args := composeArgs(project, operation...)
	if r.contexts != nil && r.contexts.ContextName() != "" {
		args = append([]string{"--context", r.contexts.ContextName()}, args...)
	}
	return args
}
func composeArgs(project model.ComposeProject, operation ...string) []string {
	args := []string{"compose", "--project-name", project.Name}
	for _, file := range project.ConfigFiles {
		args = append(args, "--file", file)
	}
	return append(args, operation...)
}
