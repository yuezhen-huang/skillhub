package gitlab

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/yuezhen-huang/skillhub/internal/models"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
)

// RepositoryManager handles Git repository operations
type RepositoryManager struct {
	auth *http.BasicAuth
}

// NewRepositoryManager creates a new repository manager
func NewRepositoryManager() *RepositoryManager {
	return &RepositoryManager{}
}

// LoadFromPath loads repository info from an existing path
func (r *RepositoryManager) LoadFromPath(ctx context.Context, path string) (*models.Repository, error) {
	repo, err := git.PlainOpen(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open repo: %w", err)
	}

	head, err := repo.Head()
	if err != nil {
		return nil, fmt.Errorf("failed to get head: %w", err)
	}

	remote, err := repo.Remote("origin")
	var remoteURL string
	if err == nil {
		config := remote.Config()
		if len(config.URLs) > 0 {
			remoteURL = config.URLs[0]
		}
	}

	var branch, tag string
	refName := head.Name()
	if refName.IsBranch() {
		branch = refName.Short()
	} else if refName.IsTag() {
		tag = refName.Short()
	}

	now := time.Now()
	return &models.Repository{
		Path:     path,
		URL:      remoteURL,
		Remote:   "origin",
		Branch:   branch,
		Tag:      tag,
		Commit:   head.Hash().String(),
		LastPull: &now,
	}, nil
}

// Clone clones a repository from GitLab
func (r *RepositoryManager) Clone(ctx context.Context, url, path string) (*models.Repository, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}

	cloneOpts := &git.CloneOptions{
		URL:      url,
		Progress: os.Stdout,
	}

	if r.auth != nil {
		cloneOpts.Auth = r.auth
	}

	repo, err := git.PlainCloneContext(ctx, path, false, cloneOpts)
	if err != nil {
		return nil, fmt.Errorf("clone failed: %w", err)
	}

	head, err := repo.Head()
	if err != nil {
		return nil, err
	}

	refName := head.Name()
	var branch, tag string
	if refName.IsBranch() {
		branch = refName.Short()
	} else if refName.IsTag() {
		tag = refName.Short()
	}

	now := time.Now()
	return &models.Repository{
		URL:      url,
		Remote:   "origin",
		Path:     path,
		Branch:   branch,
		Tag:      tag,
		Commit:   head.Hash().String(),
		LastPull: &now,
	}, nil
}

// Pull pulls the latest changes from the remote
func (r *RepositoryManager) Pull(ctx context.Context, repo *models.Repository) error {
	gitRepo, err := git.PlainOpen(repo.Path)
	if err != nil {
		return err
	}

	worktree, err := gitRepo.Worktree()
	if err != nil {
		return err
	}

	pullOpts := &git.PullOptions{
		RemoteName: repo.Remote,
	}

	if r.auth != nil {
		pullOpts.Auth = r.auth
	}

	if err := worktree.PullContext(ctx, pullOpts); err != nil && err != git.NoErrAlreadyUpToDate {
		return err
	}

	head, err := gitRepo.Head()
	if err != nil {
		return err
	}

	now := time.Now()
	repo.Commit = head.Hash().String()
	repo.LastPull = &now

	return nil
}

// CheckoutBranch checks out a specific branch
func (r *RepositoryManager) CheckoutBranch(ctx context.Context, repo *models.Repository, branch string) error {
	gitRepo, err := git.PlainOpen(repo.Path)
	if err != nil {
		return err
	}

	worktree, err := gitRepo.Worktree()
	if err != nil {
		return err
	}

	refName := plumbing.NewBranchReferenceName(branch)
	checkoutOpts := &git.CheckoutOptions{
		Branch: refName,
		Create: false,
	}

	if err := worktree.Checkout(checkoutOpts); err != nil {
		// Try to create the branch from remote
		remoteRef := plumbing.NewRemoteReferenceName("origin", branch)
		refs, err := gitRepo.References()
		if err != nil {
			return err
		}
		defer refs.Close()

		var targetHash plumbing.Hash
		found := false
		refs.ForEach(func(ref *plumbing.Reference) error {
			if ref.Name() == remoteRef {
				targetHash = ref.Hash()
				found = true
			}
			return nil
		})

		if !found {
			return fmt.Errorf("branch %s not found", branch)
		}

		checkoutOpts.Create = true
		checkoutOpts.Hash = targetHash
		checkoutOpts.Branch = refName

		if err := worktree.Checkout(checkoutOpts); err != nil {
			return err
		}
	}

	head, err := gitRepo.Head()
	if err != nil {
		return err
	}

	repo.Branch = branch
	repo.Tag = ""
	repo.Commit = head.Hash().String()

	return nil
}

// CheckoutTag checks out a specific tag
func (r *RepositoryManager) CheckoutTag(ctx context.Context, repo *models.Repository, tag string) error {
	gitRepo, err := git.PlainOpen(repo.Path)
	if err != nil {
		return err
	}

	worktree, err := gitRepo.Worktree()
	if err != nil {
		return err
	}

	tagRef := plumbing.NewTagReferenceName(tag)
	ref, err := gitRepo.Reference(tagRef, true)
	if err != nil {
		return fmt.Errorf("tag %s not found: %w", tag, err)
	}

	var hash plumbing.Hash
	if ref.Type() == plumbing.SymbolicReference {
		// For symbolic references, we need to resolve recursively
		targetRef, err := gitRepo.Reference(ref.Target(), true)
		if err == nil {
			hash = targetRef.Hash()
		} else {
			hash = ref.Hash()
		}
	} else {
		hash = ref.Hash()
	}

	// Check if it's an annotated tag
	if tagObj, err := gitRepo.TagObject(hash); err == nil {
		hash = tagObj.Target
	}

	checkoutOpts := &git.CheckoutOptions{
		Hash: hash,
	}

	if err := worktree.Checkout(checkoutOpts); err != nil {
		return err
	}

	repo.Branch = ""
	repo.Tag = tag
	repo.Commit = hash.String()

	return nil
}

// CheckoutCommit checks out a specific commit
func (r *RepositoryManager) CheckoutCommit(ctx context.Context, repo *models.Repository, commit string) error {
	gitRepo, err := git.PlainOpen(repo.Path)
	if err != nil {
		return err
	}

	worktree, err := gitRepo.Worktree()
	if err != nil {
		return err
	}

	hash := plumbing.NewHash(commit)
	checkoutOpts := &git.CheckoutOptions{
		Hash: hash,
	}

	if err := worktree.Checkout(checkoutOpts); err != nil {
		return err
	}

	repo.Branch = ""
	repo.Tag = ""
	repo.Commit = commit

	return nil
}

// ListTags lists all tags in the repository
func (r *RepositoryManager) ListTags(ctx context.Context, repo *models.Repository) ([]string, error) {
	gitRepo, err := git.PlainOpen(repo.Path)
	if err != nil {
		return nil, err
	}

	tags, err := gitRepo.Tags()
	if err != nil {
		return nil, err
	}
	defer tags.Close()

	var tagNames []string
	tags.ForEach(func(ref *plumbing.Reference) error {
		if ref.Name().IsTag() {
			tagNames = append(tagNames, ref.Name().Short())
		}
		return nil
	})

	return tagNames, nil
}

// ListBranches lists all branches in the repository
func (r *RepositoryManager) ListBranches(ctx context.Context, repo *models.Repository) ([]string, error) {
	gitRepo, err := git.PlainOpen(repo.Path)
	if err != nil {
		return nil, err
	}

	refs, err := gitRepo.References()
	if err != nil {
		return nil, err
	}
	defer refs.Close()

	var branches []string
	seen := make(map[string]bool)
	refs.ForEach(func(ref *plumbing.Reference) error {
		if ref.Name().IsBranch() {
			name := ref.Name().Short()
			if !seen[name] {
				seen[name] = true
				branches = append(branches, name)
			}
		}
		return nil
	})

	return branches, nil
}

// GetCurrentCommit gets the current commit hash
func (r *RepositoryManager) GetCurrentCommit(ctx context.Context, repo *models.Repository) (string, error) {
	gitRepo, err := git.PlainOpen(repo.Path)
	if err != nil {
		return "", err
	}

	head, err := gitRepo.Head()
	if err != nil {
		return "", err
	}

	return head.Hash().String(), nil
}

// Fetch fetches all refs from remote
func (r *RepositoryManager) Fetch(ctx context.Context, repo *models.Repository) error {
	gitRepo, err := git.PlainOpen(repo.Path)
	if err != nil {
		return err
	}

	fetchOpts := &git.FetchOptions{
		RemoteName: repo.Remote,
		RefSpecs:   []config.RefSpec{"refs/*:refs/*", "HEAD:refs/heads/HEAD"},
	}

	if r.auth != nil {
		fetchOpts.Auth = r.auth
	}

	if err := gitRepo.FetchContext(ctx, fetchOpts); err != nil && err != git.NoErrAlreadyUpToDate {
		return err
	}

	return nil
}

// GetCommitTime gets the time of a commit
func (r *RepositoryManager) GetCommitTime(path, commitHash string) (time.Time, error) {
	gitRepo, err := git.PlainOpen(path)
	if err != nil {
		return time.Time{}, err
	}

	hash := plumbing.NewHash(commitHash)
	commit, err := gitRepo.CommitObject(hash)
	if err != nil {
		return time.Time{}, err
	}

	return commit.Committer.When, nil
}

// GetLatestTag gets the latest tag by commit time
func (r *RepositoryManager) GetLatestTag(path string) (string, error) {
	gitRepo, err := git.PlainOpen(path)
	if err != nil {
		return "", err
	}

	tags, err := gitRepo.Tags()
	if err != nil {
		return "", err
	}
	defer tags.Close()

	type tagInfo struct {
		name string
		when time.Time
	}

	var tagList []tagInfo
	tags.ForEach(func(ref *plumbing.Reference) error {
		if !ref.Name().IsTag() {
			return nil
		}

		hash := ref.Hash()
		if ref.Type() == plumbing.SymbolicReference {
			// For symbolic references, try to resolve them
			targetRef, err := gitRepo.Reference(ref.Target(), true)
			if err == nil {
				hash = targetRef.Hash()
			}
		}

		commit, err := gitRepo.CommitObject(hash)
		if err != nil {
			// Try to get it as an annotated tag first
			if tagObj, err := gitRepo.TagObject(ref.Hash()); err == nil {
				commit, _ = gitRepo.CommitObject(tagObj.Target)
			}
		}

		if commit != nil {
			tagList = append(tagList, tagInfo{
				name: ref.Name().Short(),
				when: commit.Committer.When,
			})
		}
		return nil
	})

	if len(tagList) == 0 {
		return "", fmt.Errorf("no tags found")
	}

	latest := tagList[0]
	for _, t := range tagList[1:] {
		if t.when.After(latest.when) {
			latest = t
		}
	}

	return latest.name, nil
}
