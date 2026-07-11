package blueprint

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerificationBlueprintFixturesHaveNoUTF8BOM(t *testing.T) {
	root := filepath.Join("..", "..", "..", "examples", "verification-blueprints")
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !isVerificationFixtureJSON(path) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if len(data) >= 3 && data[0] == 0xef && data[1] == 0xbb && data[2] == 0xbf {
			t.Errorf("%s starts with a UTF-8 BOM, which JSON.parse rejects", path)
		}
		if !json.Valid(data) {
			t.Errorf("%s is not valid UTF-8 JSON", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk verification fixtures: %v", err)
	}
}

func isVerificationFixtureJSON(path string) bool {
	extension := strings.ToLower(filepath.Ext(path))
	return extension == ".vgf" || extension == ".obp" || extension == ".obpf" || extension == ".json"
}
