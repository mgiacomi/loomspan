package main

import (
	"fmt"
	"path/filepath"
	"runtime"
)

type projectPaths struct {
	repository  string
	module      string
	web         string
	generated   string
	build       string
	dist        string
	release     string
	agentSkills string
}

func resolveProjectPaths() (projectPaths, error) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		return projectPaths{}, fmt.Errorf("resolve build tool source location")
	}
	module, err := filepath.Abs(filepath.Join(filepath.Dir(source), "..", ".."))
	if err != nil {
		return projectPaths{}, err
	}
	return projectPaths{
		repository:  filepath.Dir(module),
		module:      module,
		web:         filepath.Join(module, "web"),
		generated:   filepath.Join(module, "internal", "webassets", "generated"),
		build:       filepath.Join(module, "build"),
		dist:        filepath.Join(module, "dist"),
		release:     filepath.Join(module, "release"),
		agentSkills: filepath.Join(module, "agent-skills"),
	}, nil
}
