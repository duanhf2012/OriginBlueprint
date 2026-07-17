package blueprint

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func verificationAssetPaths() []string {
	return []string{
		"01_legacy_all_nodes_showcase.vgf",
		"02_control_flow_maze.obp",
		"03_array_data_lab.obp",
		"04_deterministic_algorithm.obp",
		"05_function_orchestrator.obp",
		"06_async_delay_resume.obp",
		"07_async_rpc_resume_to.obp",
		"functions/10_score_kernel.obpf",
		"functions/11_array_fold_and_format.obpf",
		"functions/12_nested_control_function.obpf",
		"functions/13_local_state_isolation.obpf",
		"functions/14_async_delay_function.obpf",
		"functions/15_variable_types_lifecycle.obpf",
	}
}

func TestVerificationAssetsHaveDifferentialImplementations(t *testing.T) {
	root := filepath.Join("..", "..", "..", "examples", "verification-blueprints")
	var files []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !isVerificationGraphFixture(path) {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(relative))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)

	implemented := verificationAssetPaths()
	sort.Strings(implemented)
	if !reflect.DeepEqual(implemented, files) {
		t.Fatalf("differential asset coverage mismatch\nimplemented: %s\nfixtures: %s", strings.Join(implemented, ", "), strings.Join(files, ", "))
	}
}
