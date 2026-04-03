package spec

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// FromGit clones a remote git repository to a temp directory and loads the
// OpenAPI spec at specPath (relative to the repo root).
//
// repoURL may optionally include a branch via a fragment: https://github.com/org/repo#mybranch
func FromGit(repoURL, specPath string) (*LoadedSpec, error) {
	// Split optional branch fragment from URL.
	branch := ""
	if idx := strings.Index(repoURL, "#"); idx != -1 {
		branch = repoURL[idx+1:]
		repoURL = repoURL[:idx]
	}

	dir, err := os.MkdirTemp("", "curlx-spec-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(dir)

	cloneOpts := &git.CloneOptions{
		URL:   repoURL,
		Depth: 1,
	}
	if branch != "" {
		cloneOpts.ReferenceName = plumbing.NewBranchReferenceName(branch)
		cloneOpts.SingleBranch = true
	}

	if _, err := git.PlainClone(dir, false, cloneOpts); err != nil {
		return nil, fmt.Errorf("cloning %s: %w", repoURL, err)
	}

	fullPath := filepath.Join(dir, filepath.FromSlash(specPath))
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, fmt.Errorf("reading spec at %s: %w", specPath, err)
	}

	return parse(data)
}
