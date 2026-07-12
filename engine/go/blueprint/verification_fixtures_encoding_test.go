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

func TestVerificationBlueprintFixturesConnectEveryNode(t *testing.T) {
	root := filepath.Join("..", "..", "..", "examples", "verification-blueprints")
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !isVerificationGraphFixture(path) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var document verificationFixtureDocument
		if err := json.Unmarshal(data, &document); err != nil {
			return err
		}
		connected := map[string]bool{}
		for _, connection := range document.Connections {
			connected[connection.Source] = true
			connected[connection.Target] = true
		}
		for _, edge := range document.Edges {
			connected[edge.Source] = true
			connected[edge.Target] = true
		}
		for _, node := range document.Nodes {
			if !connected[node.ID] {
				t.Errorf("%s node %s has no data or execution connection", path, node.ID)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk verification graph fixtures: %v", err)
	}
}

func TestVerificationBlueprintFixturesLoadThroughEngine(t *testing.T) {
	root := filepath.Join("..", "..", "..", "examples", "verification-blueprints")
	graphs, err := loadGraphDir(verificationFixtureRegistry(t), root)
	if err != nil {
		t.Fatalf("load verification fixtures: %v", err)
	}
	for _, name := range []string{
		"01_legacy_all_nodes_showcase",
		"确定性评分算法",
		"函数编排主图",
		"新定时器生命周期",
		"评分核心",
		"数组折叠与格式化",
		"嵌套控制流",
		"局部状态隔离",
	} {
		if graphs[name] == nil {
			t.Errorf("loaded fixtures do not contain graph %q", name)
		}
	}
}

type verificationFixtureDocument struct {
	Nodes []struct {
		ID string `json:"id"`
	} `json:"nodes"`
	Connections []struct {
		Source string `json:"source"`
		Target string `json:"target"`
	} `json:"connections"`
	Edges []struct {
		Source string `json:"source_node_id"`
		Target string `json:"des_node_id"`
	} `json:"edges"`
}

func isVerificationFixtureJSON(path string) bool {
	extension := strings.ToLower(filepath.Ext(path))
	return extension == ".vgf" || extension == ".obp" || extension == ".obpf" || extension == ".json"
}

func isVerificationGraphFixture(path string) bool {
	extension := strings.ToLower(filepath.Ext(path))
	return extension == ".vgf" || extension == ".obp" || extension == ".obpf"
}
