package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// setupTestRepo creates a temporary git repository for testing
func setupTestRepo(t *testing.T) string {
	t.Helper()

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "okfw-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to init git repo: %v", err)
	}

	// Configure git user for commits
	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = tmpDir
	cmd.Run()

	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = tmpDir
	cmd.Run()

	// Create initial commit on main branch
	cmd = exec.Command("git", "checkout", "-b", "main")
	cmd.Dir = tmpDir
	cmd.Run()

	// Create a file and commit it
	testFile := filepath.Join(tmpDir, "README.md")
	if err := os.WriteFile(testFile, []byte("# Test Repo\n"), 0644); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create test file: %v", err)
	}

	cmd = exec.Command("git", "add", "-A")
	cmd.Dir = tmpDir
	cmd.Run()

	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create initial commit: %v", err)
	}

	return tmpDir
}

// cleanupTestRepo removes the temporary repository
func cleanupTestRepo(t *testing.T, path string) {
	t.Helper()
	os.RemoveAll(path)
}

func TestIsGitRepo(t *testing.T) {
	// Test with a real git repo
	repoDir := setupTestRepo(t)
	defer cleanupTestRepo(t, repoDir)

	if !isGitRepo(repoDir) {
		t.Error("Expected isGitRepo to return true for valid repo")
	}

	// Test with non-git directory
	tmpDir, _ := os.MkdirTemp("", "not-git-*")
	defer os.RemoveAll(tmpDir)

	if isGitRepo(tmpDir) {
		t.Error("Expected isGitRepo to return false for non-git directory")
	}
}

func TestHasUncommittedChanges(t *testing.T) {
	repoDir := setupTestRepo(t)
	defer cleanupTestRepo(t, repoDir)

	// Clean repo should have no changes
	hasChanges, err := hasUncommittedChanges(repoDir)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if hasChanges {
		t.Error("Expected no uncommitted changes in clean repo")
	}

	// Create a new file (untracked)
	newFile := filepath.Join(repoDir, "new-file.md")
	if err := os.WriteFile(newFile, []byte("new content"), 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	hasChanges, err = hasUncommittedChanges(repoDir)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !hasChanges {
		t.Error("Expected uncommitted changes after creating file")
	}

	// Stage and commit the file
	cmd := exec.Command("git", "add", "-A")
	cmd.Dir = repoDir
	cmd.Run()

	cmd = exec.Command("git", "commit", "-m", "Add file")
	cmd.Dir = repoDir
	cmd.Run()

	// Should be clean again
	hasChanges, err = hasUncommittedChanges(repoDir)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if hasChanges {
		t.Error("Expected no uncommitted changes after commit")
	}

	// Modify a file (staged change)
	if err := os.WriteFile(newFile, []byte("modified content"), 0644); err != nil {
		t.Fatalf("Failed to modify file: %v", err)
	}

	hasChanges, err = hasUncommittedChanges(repoDir)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !hasChanges {
		t.Error("Expected uncommitted changes after modifying file")
	}
}

func TestBranchExists(t *testing.T) {
	repoDir := setupTestRepo(t)
	defer cleanupTestRepo(t, repoDir)

	// main branch should exist
	exists, err := branchExists(repoDir, "main")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !exists {
		t.Error("Expected 'main' branch to exist")
	}

	// nonexistent branch should not exist
	exists, err = branchExists(repoDir, "nonexistent-branch")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if exists {
		t.Error("Expected 'nonexistent-branch' to not exist")
	}
}

func TestGetCurrentBranch(t *testing.T) {
	repoDir := setupTestRepo(t)
	defer cleanupTestRepo(t, repoDir)

	branch, err := getCurrentBranch(repoDir)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if branch != "main" {
		t.Errorf("Expected branch 'main', got '%s'", branch)
	}

	// Create and switch to new branch
	cmd := exec.Command("git", "checkout", "-b", "feature-branch")
	cmd.Dir = repoDir
	cmd.Run()

	branch, err = getCurrentBranch(repoDir)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if branch != "feature-branch" {
		t.Errorf("Expected branch 'feature-branch', got '%s'", branch)
	}
}

func TestIsDateBranch(t *testing.T) {
	tests := []struct {
		branch   string
		expected bool
	}{
		{"20260213", true},
		{"20261231", true},
		{"20260101", true},
		{"main", false},
		{"master", false},
		{"feature-branch", false},
		{"2026021", false},    // Too short
		{"202602131", false},  // Too long
		{"2026-02-13", false}, // Wrong format
		{"abcdefgh", false},   // Not a date
		{"20261301", false},   // Invalid month
		{"20260230", false},   // Invalid day (Feb 30)
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.branch, func(t *testing.T) {
			result := isDateBranch(tt.branch)
			if result != tt.expected {
				t.Errorf("isDateBranch(%q) = %v, expected %v", tt.branch, result, tt.expected)
			}
		})
	}
}

func TestGitCheckout(t *testing.T) {
	repoDir := setupTestRepo(t)
	defer cleanupTestRepo(t, repoDir)

	// Create a new branch
	cmd := exec.Command("git", "checkout", "-b", "test-branch")
	cmd.Dir = repoDir
	cmd.Run()

	// Checkout main
	err := gitCheckout(repoDir, "main")
	if err != nil {
		t.Fatalf("Failed to checkout main: %v", err)
	}

	branch, _ := getCurrentBranch(repoDir)
	if branch != "main" {
		t.Errorf("Expected to be on 'main', got '%s'", branch)
	}

	// Try to checkout nonexistent branch
	err = gitCheckout(repoDir, "nonexistent")
	if err == nil {
		t.Error("Expected error when checking out nonexistent branch")
	}
}

func TestGitCheckoutNewBranch(t *testing.T) {
	repoDir := setupTestRepo(t)
	defer cleanupTestRepo(t, repoDir)

	// Create new branch
	err := gitCheckoutNewBranch(repoDir, "new-feature")
	if err != nil {
		t.Fatalf("Failed to create new branch: %v", err)
	}

	branch, _ := getCurrentBranch(repoDir)
	if branch != "new-feature" {
		t.Errorf("Expected to be on 'new-feature', got '%s'", branch)
	}

	// Try to create same branch again (should fail)
	err = gitCheckout(repoDir, "main")
	if err != nil {
		t.Fatalf("Failed to checkout main: %v", err)
	}

	err = gitCheckoutNewBranch(repoDir, "new-feature")
	if err == nil {
		t.Error("Expected error when creating existing branch")
	}
}

func TestGitAddAll(t *testing.T) {
	repoDir := setupTestRepo(t)
	defer cleanupTestRepo(t, repoDir)

	// Create a new file
	newFile := filepath.Join(repoDir, "test-file.md")
	if err := os.WriteFile(newFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	// Stage all changes
	err := gitAddAll(repoDir)
	if err != nil {
		t.Fatalf("Failed to stage changes: %v", err)
	}

	// Verify file is staged
	cmd := exec.Command("git", "diff", "--cached", "--name-only")
	cmd.Dir = repoDir
	output, _ := cmd.Output()
	if len(output) == 0 {
		t.Error("Expected staged files after git add -A")
	}
}

func TestGitCommit(t *testing.T) {
	repoDir := setupTestRepo(t)
	defer cleanupTestRepo(t, repoDir)

	// Create and stage a file
	newFile := filepath.Join(repoDir, "commit-test.md")
	if err := os.WriteFile(newFile, []byte("commit test"), 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	gitAddAll(repoDir)

	// Commit
	err := gitCommit(repoDir, "Test commit message")
	if err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Verify commit exists
	cmd := exec.Command("git", "log", "--oneline", "-1")
	cmd.Dir = repoDir
	output, _ := cmd.Output()
	if len(output) == 0 {
		t.Error("Expected commit in log")
	}
}

func TestGitMerge(t *testing.T) {
	repoDir := setupTestRepo(t)
	defer cleanupTestRepo(t, repoDir)

	// Create a feature branch with a commit
	gitCheckoutNewBranch(repoDir, "feature")
	newFile := filepath.Join(repoDir, "feature-file.md")
	os.WriteFile(newFile, []byte("feature content"), 0644)
	gitAddAll(repoDir)
	gitCommit(repoDir, "Feature commit")

	// Switch back to main and merge
	gitCheckout(repoDir, "main")

	err := gitMerge(repoDir, "feature")
	if err != nil {
		t.Fatalf("Failed to merge: %v", err)
	}

	// Verify file exists on main
	if _, err := os.Stat(newFile); os.IsNotExist(err) {
		t.Error("Expected feature file to exist after merge")
	}
}

func TestGitDeleteBranch(t *testing.T) {
	repoDir := setupTestRepo(t)
	defer cleanupTestRepo(t, repoDir)

	// Create a branch
	gitCheckoutNewBranch(repoDir, "to-delete")
	gitCheckout(repoDir, "main")

	// Delete the branch
	err := gitDeleteBranch(repoDir, "to-delete")
	if err != nil {
		t.Fatalf("Failed to delete branch: %v", err)
	}

	// Verify branch no longer exists
	exists, _ := branchExists(repoDir, "to-delete")
	if exists {
		t.Error("Expected branch to be deleted")
	}
}

// Integration tests for hello/goodbye workflow

func TestHelloWorkflow_NormalFlow(t *testing.T) {
	repoDir := setupTestRepo(t)
	defer cleanupTestRepo(t, repoDir)

	// Simulate hello command logic
	todayBranch := time.Now().Format("20060102")

	// Check no uncommitted changes
	hasChanges, _ := hasUncommittedChanges(repoDir)
	if hasChanges {
		t.Fatal("Test repo should be clean")
	}

	// Check branch doesn't exist
	exists, _ := branchExists(repoDir, todayBranch)
	if exists {
		t.Fatalf("Branch %s should not exist yet", todayBranch)
	}

	// Create the dated branch
	err := gitCheckoutNewBranch(repoDir, todayBranch)
	if err != nil {
		t.Fatalf("Failed to create dated branch: %v", err)
	}

	// Verify we're on the dated branch
	branch, _ := getCurrentBranch(repoDir)
	if branch != todayBranch {
		t.Errorf("Expected to be on '%s', got '%s'", todayBranch, branch)
	}

	// Verify branch is a date branch
	if !isDateBranch(branch) {
		t.Errorf("Branch '%s' should be recognized as a date branch", branch)
	}
}

func TestHelloWorkflow_BranchAlreadyExists(t *testing.T) {
	repoDir := setupTestRepo(t)
	defer cleanupTestRepo(t, repoDir)

	todayBranch := time.Now().Format("20060102")

	// Pre-create the dated branch
	err := gitCheckoutNewBranch(repoDir, todayBranch)
	if err != nil {
		t.Fatalf("Failed to create dated branch: %v", err)
	}

	// Switch back to main
	gitCheckout(repoDir, "main")

	// Now check if branch exists (simulating hello command check)
	exists, _ := branchExists(repoDir, todayBranch)
	if !exists {
		t.Fatal("Branch should exist")
	}

	// In real command, this would warn and return early
	// We just verify the check works
}

func TestHelloWorkflow_UncommittedChanges(t *testing.T) {
	repoDir := setupTestRepo(t)
	defer cleanupTestRepo(t, repoDir)

	// Create uncommitted changes
	newFile := filepath.Join(repoDir, "uncommitted.md")
	os.WriteFile(newFile, []byte("uncommitted content"), 0644)

	// Check for uncommitted changes (simulating hello command check)
	hasChanges, _ := hasUncommittedChanges(repoDir)
	if !hasChanges {
		t.Fatal("Should detect uncommitted changes")
	}

	// In real command, this would warn and return early
	// We just verify the check works
}

func TestGoodbyeWorkflow_NormalFlow(t *testing.T) {
	repoDir := setupTestRepo(t)
	defer cleanupTestRepo(t, repoDir)

	todayBranch := time.Now().Format("20060102")

	// Simulate hello: create dated branch
	gitCheckoutNewBranch(repoDir, todayBranch)

	// Create some changes
	newFile := filepath.Join(repoDir, "notes.md")
	os.WriteFile(newFile, []byte("Today's notes"), 0644)

	// Verify on dated branch
	branch, _ := getCurrentBranch(repoDir)
	if !isDateBranch(branch) {
		t.Fatal("Should be on a dated branch")
	}

	// Simulate goodbye: stage and commit
	hasChanges, _ := hasUncommittedChanges(repoDir)
	if !hasChanges {
		t.Fatal("Should have changes to commit")
	}

	gitAddAll(repoDir)
	gitCommit(repoDir, "Notes for "+todayBranch)

	// Switch to main and merge
	gitCheckout(repoDir, "main")
	err := gitMerge(repoDir, todayBranch)
	if err != nil {
		t.Fatalf("Failed to merge: %v", err)
	}

	// Delete the dated branch
	err = gitDeleteBranch(repoDir, todayBranch)
	if err != nil {
		t.Fatalf("Failed to delete branch: %v", err)
	}

	// Verify file exists on main
	if _, err := os.Stat(newFile); os.IsNotExist(err) {
		t.Error("Notes file should exist on main after merge")
	}

	// Verify branch is deleted
	exists, _ := branchExists(repoDir, todayBranch)
	if exists {
		t.Error("Dated branch should be deleted")
	}
}

func TestGoodbyeWorkflow_NotOnDateBranch(t *testing.T) {
	repoDir := setupTestRepo(t)
	defer cleanupTestRepo(t, repoDir)

	// Stay on main branch
	branch, _ := getCurrentBranch(repoDir)
	if branch != "main" {
		t.Fatalf("Expected to be on main, got '%s'", branch)
	}

	// Check if it's a dated branch (simulating goodbye command check)
	if isDateBranch(branch) {
		t.Error("main should not be recognized as a date branch")
	}

	// Create a non-date feature branch
	gitCheckoutNewBranch(repoDir, "feature-xyz")
	branch, _ = getCurrentBranch(repoDir)
	if isDateBranch(branch) {
		t.Error("feature-xyz should not be recognized as a date branch")
	}

	// In real command, this would warn and return early
}

func TestGoodbyeWorkflow_NoChangesToCommit(t *testing.T) {
	repoDir := setupTestRepo(t)
	defer cleanupTestRepo(t, repoDir)

	todayBranch := time.Now().Format("20060102")

	// Create dated branch but don't make any changes
	gitCheckoutNewBranch(repoDir, todayBranch)

	// Check for changes
	hasChanges, _ := hasUncommittedChanges(repoDir)
	if hasChanges {
		t.Error("Should have no uncommitted changes")
	}

	// Still should be able to merge (even with no new commits)
	gitCheckout(repoDir, "main")
	err := gitMerge(repoDir, todayBranch)
	if err != nil {
		t.Fatalf("Merge should succeed even with no changes: %v", err)
	}
}
