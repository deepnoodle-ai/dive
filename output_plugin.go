package dive

import (
	"context"
	"os"
	"path/filepath"
)

// OutputPlugin is a plugin that can be used to store and retrieve task outputs
type OutputPlugin interface {
	// Name of the output plugin
	Name() string

	// OutputExists returns true if the output for the given task and fingerprint
	// already exists.
	OutputExists(ctx context.Context, name, fingerprint string) (bool, error)

	// ReadOutput reads the output for the given task and fingerprint
	ReadOutput(ctx context.Context, name, fingerprint string) (string, error)

	// WriteOutput writes the output for the given task and fingerprint
	WriteOutput(ctx context.Context, name, fingerprint string, output string) error
}

type DiskOutputPlugin struct {
	dir string
}

func NewDiskOutputPlugin(dir string) (*DiskOutputPlugin, error) {
	if dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, err
		}
	}
	return &DiskOutputPlugin{dir: dir}, nil
}

func (p *DiskOutputPlugin) Name() string {
	return "disk"
}

func (p *DiskOutputPlugin) OutputExists(ctx context.Context, name, fingerprint string) (bool, error) {
	path := p.getPath(name, fingerprint)
	_, err := os.Stat(path)
	return err == nil, nil
}

func (p *DiskOutputPlugin) ReadOutput(ctx context.Context, name, fingerprint string) (string, error) {
	path := p.getPath(name, fingerprint)
	content, err := os.ReadFile(path)
	return string(content), err
}

func (p *DiskOutputPlugin) WriteOutput(ctx context.Context, name, fingerprint string, output string) error {
	path := p.getPath(name, fingerprint)
	return os.WriteFile(path, []byte(output), 0644)
}

func (p *DiskOutputPlugin) getPath(name, fingerprint string) string {
	if fingerprint == "" {
		return filepath.Join(p.dir, name+".md")
	} else {
		return filepath.Join(p.dir, name+"-"+fingerprint+".md")
	}
}

type InMemoryOutputPlugin struct {
	outputs map[string]string
}

func NewInMemoryOutputPlugin() *InMemoryOutputPlugin {
	return &InMemoryOutputPlugin{outputs: make(map[string]string)}
}

func (p *InMemoryOutputPlugin) Name() string {
	return "memory"
}

func (p *InMemoryOutputPlugin) OutputExists(ctx context.Context, name, fingerprint string) (bool, error) {
	_, ok := p.outputs[p.getKey(name, fingerprint)]
	return ok, nil
}

func (p *InMemoryOutputPlugin) ReadOutput(ctx context.Context, name, fingerprint string) (string, error) {
	return p.outputs[p.getKey(name, fingerprint)], nil
}

func (p *InMemoryOutputPlugin) WriteOutput(ctx context.Context, name, fingerprint string, output string) error {
	p.outputs[p.getKey(name, fingerprint)] = output
	return nil
}

func (p *InMemoryOutputPlugin) getKey(name, fingerprint string) string {
	if fingerprint == "" {
		return name
	} else {
		return name + "-" + fingerprint
	}
}
