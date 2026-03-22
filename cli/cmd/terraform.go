package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// terraformDir returns the path to the terraform workspace for a game.
// It uses the directory of this binary to locate the repo root.
func terraformDir(game string) (string, error) {
	// Walk up from the binary location to find the repo root containing terraform/
	// For development, fall back to searching from the executable's location.
	root, err := findRepoRoot()
	if err != nil {
		return "", fmt.Errorf("cannot locate repo root: %w", err)
	}
	dir := filepath.Join(root, "terraform", "games", game)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return "", fmt.Errorf("game %q not found (no terraform workspace at %s)", game, dir)
	}
	return dir, nil
}

// findRepoRoot locates the repository root by searching for a terraform/games directory.
func findRepoRoot() (string, error) {
	// Try environment variable first (useful for testing and packaged installs)
	if root := os.Getenv("BONFIRE_REPO_ROOT"); root != "" {
		return root, nil
	}

	// Walk up from the executable location
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	// Follow symlinks
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return "", err
	}

	dir := filepath.Dir(exe)
	for {
		candidate := filepath.Join(dir, "terraform", "games")
		if _, err := os.Stat(candidate); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	// Last resort: check if we're running tests or in the source tree
	_, filename, _, ok := runtime.Caller(0)
	if ok {
		// filename is something like .../cli/cmd/terraform.go
		// repo root is three levels up
		root := filepath.Join(filepath.Dir(filename), "..", "..")
		candidate := filepath.Join(root, "terraform", "games")
		if _, err := os.Stat(candidate); err == nil {
			return filepath.Clean(root), nil
		}
	}

	return "", fmt.Errorf("could not find repo root (set BONFIRE_REPO_ROOT to override)")
}

// terraformInit runs terraform init in the given workspace directory.
func terraformInit(dir string) error {
	return runTerraform(dir, "init")
}

// terraformApply runs terraform apply -auto-approve in the given workspace directory.
func terraformApply(dir string) error {
	return runTerraform(dir, "apply", "-auto-approve")
}

// terraformDestroy runs terraform destroy -auto-approve in the given workspace directory.
func terraformDestroy(dir string) error {
	return runTerraform(dir, "destroy", "-auto-approve")
}

// terraformPlanDestroy runs terraform plan -destroy, saving the plan to planFile.
func terraformPlanDestroy(dir, planFile string) error {
	return runTerraform(dir, "plan", "-destroy", "-out="+planFile)
}

// terraformApplyPlan runs terraform apply with a saved plan file.
func terraformApplyPlan(dir, planFile string) error {
	return runTerraform(dir, "apply", planFile)
}

// terraformOutput returns a map of terraform output values for the given workspace.
func terraformOutput(dir string) (map[string]terraformOutputValue, error) {
	cmd := exec.Command("terraform", "output", "-json")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("terraform output: %w", err)
	}
	var outputs map[string]terraformOutputValue
	if err := json.Unmarshal(out, &outputs); err != nil {
		return nil, fmt.Errorf("parsing terraform output: %w", err)
	}
	return outputs, nil
}

type terraformOutputValue struct {
	Value interface{} `json:"value"`
	Type  interface{} `json:"type"`
}

func (v terraformOutputValue) String() string {
	if s, ok := v.Value.(string); ok {
		return s
	}
	b, _ := json.Marshal(v.Value)
	return string(b)
}

// runTerraform executes a terraform command in the given directory, streaming
// stdout/stderr to the terminal.
func runTerraform(dir string, args ...string) error {
	cmd := exec.Command("terraform", args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("terraform %v: %w", args, err)
	}
	return nil
}

// availableGames returns the list of games that have a terraform workspace.
func availableGames() ([]string, error) {
	root, err := findRepoRoot()
	if err != nil {
		return nil, err
	}
	gamesDir := filepath.Join(root, "terraform", "games")
	entries, err := os.ReadDir(gamesDir)
	if err != nil {
		return nil, fmt.Errorf("reading games directory: %w", err)
	}
	var games []string
	for _, e := range entries {
		if e.IsDir() {
			games = append(games, e.Name())
		}
	}
	return games, nil
}
