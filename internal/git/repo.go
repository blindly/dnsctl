package git

import (
	"fmt"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

type Repo struct {
	repo *gogit.Repository
	path string
}

type LogEntry struct {
	Hash    string
	Message string
	When    time.Time
}

type StatusResult struct {
	clean bool
	files map[string]string
}

func (s *StatusResult) IsClean() bool {
	return s.clean
}

func (s *StatusResult) Files() map[string]string {
	return s.files
}

func Init(path string) (*Repo, error) {
	repo, err := gogit.PlainInit(path, false)
	if err != nil {
		return nil, fmt.Errorf("git init: %w", err)
	}
	return &Repo{repo: repo, path: path}, nil
}

func Open(path string) (*Repo, error) {
	repo, err := gogit.PlainOpen(path)
	if err != nil {
		return nil, fmt.Errorf("git open: %w", err)
	}
	return &Repo{repo: repo, path: path}, nil
}

func (r *Repo) AddAndCommit(message string, paths ...string) error {
	w, err := r.repo.Worktree()
	if err != nil {
		return fmt.Errorf("getting worktree: %w", err)
	}

	for _, p := range paths {
		if _, err := w.Add(p); err != nil {
			return fmt.Errorf("git add %s: %w", p, err)
		}
	}

	_, err = w.Commit(message, &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "dnsctl",
			Email: "dnsctl@localhost",
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("git commit: %w", err)
	}
	return nil
}

func (r *Repo) AddAllAndCommit(message string) error {
	w, err := r.repo.Worktree()
	if err != nil {
		return fmt.Errorf("getting worktree: %w", err)
	}

	if _, err := w.Add("."); err != nil {
		return fmt.Errorf("git add: %w", err)
	}

	_, err = w.Commit(message, &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "dnsctl",
			Email: "dnsctl@localhost",
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("git commit: %w", err)
	}
	return nil
}

func (r *Repo) Status() (*StatusResult, error) {
	w, err := r.repo.Worktree()
	if err != nil {
		return nil, fmt.Errorf("getting worktree: %w", err)
	}

	status, err := w.Status()
	if err != nil {
		return nil, fmt.Errorf("git status: %w", err)
	}

	files := make(map[string]string)
	for file, s := range status {
		code := string(s.Worktree)
		if s.Staging != ' ' && s.Staging != '?' {
			code = string(s.Staging)
		}
		files[file] = code
	}

	return &StatusResult{
		clean: status.IsClean(),
		files: files,
	}, nil
}

func (r *Repo) Log(max int) ([]LogEntry, error) {
	iter, err := r.repo.Log(&gogit.LogOptions{})
	if err != nil {
		return nil, fmt.Errorf("git log: %w", err)
	}

	var entries []LogEntry
	count := 0
	err = iter.ForEach(func(c *object.Commit) error {
		if count >= max {
			return fmt.Errorf("stop")
		}
		entries = append(entries, LogEntry{
			Hash:    c.Hash.String()[:7],
			Message: c.Message,
			When:    c.Author.When,
		})
		count++
		return nil
	})
	// The "stop" error is expected for limiting
	if err != nil && err.Error() != "stop" {
		return nil, err
	}

	return entries, nil
}
