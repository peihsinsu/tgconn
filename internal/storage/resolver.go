// Package storage resolves and manages the per-project state directory tgconn
// uses for daily logs, history, downloaded media, and cron jobs.
//
// All per-project state lives under ~/.tgconn/projects/<encoded-cwd>/, where
// <encoded-cwd> is the absolute working directory with every "/" replaced by
// "-" (Claude Code's encoding scheme).
package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	rootDirName     = ".tgconn"
	projectsDirName = "projects"
	legacyDirName   = ".tgconn"
)

// EncodeCWD encodes an absolute working directory into a flat identifier by
// replacing every "/" with "-". Mirrors Claude Code's encoding scheme.
func EncodeCWD(cwd string) string {
	return strings.ReplaceAll(cwd, "/", "-")
}

// Resolver answers path queries for the current project.
type Resolver struct {
	cwd     string
	home    string
	project string
}

// NewResolver builds a Resolver from the current working directory and user home.
func NewResolver() (*Resolver, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("get cwd: %w", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get user home: %w", err)
	}
	return newResolver(cwd, home), nil
}

func newResolver(cwd, home string) *Resolver {
	return &Resolver{
		cwd:     cwd,
		home:    home,
		project: filepath.Join(home, rootDirName, projectsDirName, EncodeCWD(cwd)),
	}
}

// CWD returns the working directory the resolver was created from.
func (r *Resolver) CWD() string { return r.cwd }

// ProjectDir returns the centralized per-project storage path.
func (r *Resolver) ProjectDir() string { return r.project }

// Subdir joins name onto the project directory.
func (r *Resolver) Subdir(name string) string {
	return filepath.Join(r.project, name)
}

// LegacyDir returns the legacy <cwd>/.tgconn/ path (used by migration only).
func (r *Resolver) LegacyDir() string {
	return filepath.Join(r.cwd, legacyDirName)
}
