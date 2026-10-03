package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func writeTestCSV(t *testing.T, path string, lines []string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644); err != nil {
		t.Fatal(err)
	}
}

func writeTestXlsx(t *testing.T, path string, sheets map[string][][]string) {
	t.Helper()
	workbook := excelize.NewFile()
	for sheet, rows := range sheets {
		if _, err := workbook.NewSheet(sheet); err != nil {
			t.Fatal(err)
		}
		for row, cells := range rows {
			for column, cell := range cells {
				coordinate, err := excelize.CoordinatesToCellName(column+1, row+1)
				if err != nil {
					t.Fatal(err)
				}
				if err := workbook.SetCellValue(sheet, coordinate, cell); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	if err := workbook.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	if err := workbook.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestLoadConfigTablesCSVWithDefaultRules(t *testing.T) {
	t.Setenv("ORIGIN_BLUEPRINT_CONFIG_PATH", filepath.Join(t.TempDir(), "config.json"))
	app := NewApp()
	workspace := t.TempDir()
	configs := filepath.Join(workspace, "configs")
	if err := os.MkdirAll(configs, 0755); err != nil {
		t.Fatal(err)
	}
	writeTestCSV(t, filepath.Join(configs, "skills.csv"), []string{
		"id,名称,备注",
		"100233,火球术,主火技能",
		"100234,冰霜箭",
		",空行跳过",
		"100235,雷霆一击",
		"int,string,类型行跳过",
		"#注释,注释行跳过",
	})
	tables, err := app.LoadConfigTables(workspace, ConfigTablesSettings{
		Datasets: []ConfigTableDataset{{Key: "skills", Name: "技能表", File: "skills.csv"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(tables) != 1 {
		t.Fatalf("tables = %#v", tables)
	}
	table := tables[0]
	if table.Key != "skills" || table.Name != "技能表" || table.Warning != "" || len(table.Entries) != 3 {
		t.Fatalf("table = %#v", table)
	}
	if table.Entries[0].ID != "100233" || table.Entries[0].Name != "火球术" {
		t.Fatalf("entries = %#v", table.Entries)
	}
}

func TestLoadConfigTablesXlsxMultiRowHeaderAndOverrides(t *testing.T) {
	t.Setenv("ORIGIN_BLUEPRINT_CONFIG_PATH", filepath.Join(t.TempDir(), "config.json"))
	app := NewApp()
	workspace := t.TempDir()
	configs := filepath.Join(workspace, "configs")
	if err := os.MkdirAll(configs, 0755); err != nil {
		t.Fatal(err)
	}
	writeTestXlsx(t, filepath.Join(configs, "skills.xlsx"), map[string][][]string{
		"skills": {
			{"##注释", "", ""},
			{"skill_id", "技能名", "类型"},
			{"int", "string", "string"},
			{"2001", "回风斩", "主动"},
			{"2002", "破甲击", "被动"},
		},
	})
	settings := ConfigTablesSettings{
		Defaults: ConfigTableDefaults{IDMatchRow: 1, NameMatchRow: 1, IDKeyword: "id", NameKeywords: []string{"name", "名称"}},
		Datasets: []ConfigTableDataset{{
			Key: "skills", Name: "skills", File: "skills.xlsx", Sheet: "skills",
			IDMatchRow: 2, NameMatchRow: 2, IDKeyword: "skill_id", NameKeyword: "技能名,skillName",
		}},
	}
	tables, err := app.LoadConfigTables(workspace, settings)
	if err != nil {
		t.Fatal(err)
	}
	if len(tables) != 1 || tables[0].Key != "skills" {
		t.Fatalf("tables = %#v", tables)
	}
	table := tables[0]
	if table.Warning != "" || len(table.Entries) != 2 {
		t.Fatalf("table = %#v", table)
	}
	if table.Entries[0].ID != "2001" || table.Entries[0].Name != "回风斩" {
		t.Fatalf("entries = %#v", table.Entries)
	}
	if table.IDColumn != "A" || table.NameColumn != "B" {
		t.Fatalf("columns = %q %q", table.IDColumn, table.NameColumn)
	}
}

func TestLoadConfigTablesReportsUnidentifiedColumns(t *testing.T) {
	t.Setenv("ORIGIN_BLUEPRINT_CONFIG_PATH", filepath.Join(t.TempDir(), "config.json"))
	app := NewApp()
	workspace := t.TempDir()
	configs := filepath.Join(workspace, "configs")
	if err := os.MkdirAll(configs, 0755); err != nil {
		t.Fatal(err)
	}
	writeTestCSV(t, filepath.Join(configs, "items.csv"), []string{
		"编号,称呼",
		"1,剑",
	})
	tables, err := app.LoadConfigTables(workspace, ConfigTablesSettings{
		Datasets: []ConfigTableDataset{{Key: "items", Name: "items", File: "items.csv"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(tables) != 1 || tables[0].Entries != nil {
		t.Fatalf("tables = %#v", tables)
	}
	if tables[0].Warning == "" || !strings.Contains(tables[0].Warning, "未识别出 id 列（表头第 1 行") {
		t.Fatalf("warning = %q", tables[0].Warning)
	}
}

func TestLoadProjectSettingsMigratesLegacyProjectFile(t *testing.T) {
	t.Setenv("ORIGIN_BLUEPRINT_CONFIG_PATH", filepath.Join(t.TempDir(), "config.json"))
	app := NewApp()
	workspace := t.TempDir()
	legacy := `{"version":1,"explorer":{"revealActiveFile":true}}`
	if err := os.WriteFile(filepath.Join(workspace, legacyProjectSettingsFileName), []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}
	result, err := app.LoadProjectSettings(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(result.Path, "originblueprint.obproj") {
		t.Fatalf("path = %q", result.Path)
	}
	if result.Content != legacy {
		t.Fatalf("content = %q", result.Content)
	}
	if _, err := os.Stat(filepath.Join(workspace, "originblueprint.obproj")); err != nil {
		t.Fatalf("migrated file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspace, legacyProjectSettingsFileName)); err != nil {
		t.Fatalf("legacy file should be preserved until user removes it: %v", err)
	}
}

func TestScanAndImportWhitelist(t *testing.T) {
	t.Setenv("ORIGIN_BLUEPRINT_CONFIG_PATH", filepath.Join(t.TempDir(), "config.json"))
	app := NewApp()
	workspace := t.TempDir()
	configs := filepath.Join(workspace, "tables")
	if err := os.MkdirAll(configs, 0755); err != nil {
		t.Fatal(err)
	}
	writeTestCSV(t, filepath.Join(configs, "skills.csv"), []string{
		"id,name",
		"1,Fireball",
	})
	writeTestCSV(t, filepath.Join(configs, "items.csv"), []string{
		"id,name",
		"2,Sword",
	})
	settings := ConfigTablesSettings{
		Directories: []string{"tables"},
		Datasets:    []ConfigTableDataset{{Key: "skills", Name: "skills", File: "skills.csv"}},
	}
	scanned, err := app.ScanConfigTables(workspace, settings)
	if err != nil {
		t.Fatal(err)
	}
	if len(scanned) != 2 {
		t.Fatalf("scanned = %#v", scanned)
	}
	if !scanned[1].Imported || scanned[0].Imported {
		t.Fatalf("imported flags = %#v", scanned)
	}
	if len(scanned[0].Entries) != 0 || len(scanned[1].Entries) != 0 {
		t.Fatalf("scan should not load entries: %#v", scanned)
	}
	loaded, err := app.LoadConfigTables(workspace, settings)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || loaded[0].Key != "skills" || len(loaded[0].Entries) != 1 {
		t.Fatalf("loaded = %#v", loaded)
	}
}

func TestPreviewConfigTableAppliesDraftRules(t *testing.T) {
	t.Setenv("ORIGIN_BLUEPRINT_CONFIG_PATH", filepath.Join(t.TempDir(), "config.json"))
	app := NewApp()
	workspace := t.TempDir()
	configs := filepath.Join(workspace, "configs")
	if err := os.MkdirAll(configs, 0755); err != nil {
		t.Fatal(err)
	}
	writeTestCSV(t, filepath.Join(configs, "skills.csv"), []string{
		"##注释",
		"skill_id,技能名,类型",
		"int,string,string",
		"3001,横扫,主动",
	})
	settings := ConfigTablesSettings{
		Datasets: []ConfigTableDataset{{Key: "skills", Name: "skills", File: "skills.csv", IDMatchRow: 2, NameMatchRow: 2, IDKeyword: "skill_id", NameKeyword: "技能名"}},
	}
	table, err := app.PreviewConfigTable(workspace, settings, "skills", "类型", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(table.Entries) != 1 || table.Entries[0].ID != "3001" || table.Entries[0].Name != "横扫" {
		t.Fatalf("preview = %#v", table)
	}
	if table.Entries[0].Extra != "主动" {
		t.Fatalf("extra = %#v", table.Entries[0])
	}
	if _, err := app.PreviewConfigTable(workspace, settings, "missing", "", 0); err == nil {
		t.Fatal("preview of missing dataset should fail")
	}
	settings.Datasets[0].File = "gone.csv"
	if _, err := app.PreviewConfigTable(workspace, settings, "skills", "", 0); err == nil {
		t.Fatal("missing source file should fail preview")
	}
}

func TestConfigTableSeparateMatchRows(t *testing.T) {
	t.Setenv("ORIGIN_BLUEPRINT_CONFIG_PATH", filepath.Join(t.TempDir(), "config.json"))
	app := NewApp()
	workspace := t.TempDir()
	configs := filepath.Join(workspace, "configs")
	if err := os.MkdirAll(configs, 0755); err != nil {
		t.Fatal(err)
	}
	// id 表头在第 2 行，名称表头在第 3 行，数据从第 4 行起。
	writeTestCSV(t, filepath.Join(configs, "skills.csv"), []string{
		"##注释行",
		"skill_id,类型",
		"int,技能名",
		"4001,回旋斩",
		"4002,裂地斩",
	})
	tables, err := app.LoadConfigTables(workspace, ConfigTablesSettings{
		Datasets: []ConfigTableDataset{{Key: "skills", Name: "skills", File: "skills.csv", IDMatchRow: 2, NameMatchRow: 3, IDKeyword: "skill_id", NameKeyword: "技能名"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(tables) != 1 || tables[0].Warning != "" || len(tables[0].Entries) != 2 {
		t.Fatalf("tables = %#v", tables)
	}
	if tables[0].Entries[0].ID != "4001" || tables[0].Entries[0].Name != "回旋斩" {
		t.Fatalf("entries = %#v", tables[0].Entries)
	}
}

func TestConfigTableKeepsDuplicateAndSkipsNonNumericIds(t *testing.T) {
	t.Setenv("ORIGIN_BLUEPRINT_CONFIG_PATH", filepath.Join(t.TempDir(), "config.json"))
	app := NewApp()
	workspace := t.TempDir()
	configs := filepath.Join(workspace, "configs")
	if err := os.MkdirAll(configs, 0755); err != nil {
		t.Fatal(err)
	}
	writeTestCSV(t, filepath.Join(configs, "skills.csv"), []string{
		"id,name",
		"9001,重复A",
		"9001,重复B",
		"fire_ball,字符串id跳过",
		",空id跳过",
		"9002.5,小数保留",
	})
	tables, err := app.LoadConfigTables(workspace, ConfigTablesSettings{Datasets: []ConfigTableDataset{{Key: "skills", Name: "skills", File: "skills.csv"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(tables) != 1 || len(tables[0].Entries) != 3 {
		t.Fatalf("entries = %#v", tables[0].Entries)
	}
	if tables[0].Entries[0].ID != "9001" || tables[0].Entries[1].ID != "9001" {
		t.Fatalf("duplicate ids should be preserved: %#v", tables[0].Entries)
	}
	if tables[0].Entries[2].ID != "9002.5" {
		t.Fatalf("numeric value should be kept as-is: %#v", tables[0].Entries[2])
	}
}

// 一张物理表派生多个数据集：不同筛选各自独立成集（如同一枚举定义表切出多个枚举段）。
func TestConfigTablesOneFileMultipleDatasets(t *testing.T) {
	t.Setenv("ORIGIN_BLUEPRINT_CONFIG_PATH", filepath.Join(t.TempDir(), "config.json"))
	app := NewApp()
	workspace := t.TempDir()
	configs := filepath.Join(workspace, "configs")
	if err := os.MkdirAll(configs, 0755); err != nil {
		t.Fatal(err)
	}
	writeTestXlsx(t, filepath.Join(configs, "Buff被动枚举定义.xlsx"), map[string][][]string{
		"枚举": {
			{"id", "名称", "EnumName"},
			{"0", "无限制", "BehaviorRestrictType_NoneRestrict"},
			{"1", "禁止移动", "BehaviorRestrictType_NoMove"},
			{"0", "无免疫", "ImmuneType_None"},
			{"10", "法术伤害免疫", "ImmuneType_MagicDamage"},
		},
	})
	settings := ConfigTablesSettings{Datasets: []ConfigTableDataset{
		{
			Key: "buff_restrict", Name: "行为限制枚举", File: "Buff被动枚举定义.xlsx", Sheet: "枚举",
			Filter: &ConfigTableFilter{Mode: "text", Column: "custom", Text: "BehaviorRestrictType_", ExtraKeyword: "EnumName"},
		},
		{
			Key: "immune", Name: "免疫类型枚举", File: "Buff被动枚举定义.xlsx", Sheet: "枚举",
			Filter: &ConfigTableFilter{Mode: "text", Column: "custom", Text: "ImmuneType_", ExtraKeyword: "EnumName"},
		},
	}}
	loaded, err := app.LoadConfigTables(workspace, settings)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 2 {
		t.Fatalf("loaded = %#v", loaded)
	}
	if loaded[0].Key != "buff_restrict" || loaded[0].Name != "行为限制枚举" || len(loaded[0].Entries) != 2 {
		t.Fatalf("dataset[0] = %#v", loaded[0])
	}
	if loaded[0].Entries[1].ID != "1" || loaded[0].Entries[1].Extra != "BehaviorRestrictType_NoMove" {
		t.Fatalf("dataset[0] entries = %#v", loaded[0].Entries)
	}
	if loaded[1].Key != "immune" || len(loaded[1].Entries) != 2 || loaded[1].Entries[1].ID != "10" {
		t.Fatalf("dataset[1] = %#v", loaded[1])
	}
	scanned, err := app.ScanConfigTables(workspace, settings)
	if err != nil {
		t.Fatal(err)
	}
	if len(scanned) != 1 || !scanned[0].Imported {
		t.Fatalf("scanned = %#v", scanned)
	}
	// 预览不应用筛选，两个数据集都看到全量 4 条，便于调整筛选。
	preview, err := app.PreviewConfigTable(workspace, settings, "immune", "EnumName", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Entries) != 4 {
		t.Fatalf("preview entries = %#v", preview.Entries)
	}
}

// 旧版 configTables.tables 清单自动迁移为数据集：key 沿用旧表键（节点 ref 不受影响），
// 只存表键不存文件名的旧项按“键+.csv/.xlsx”回退匹配。
func TestLoadConfigTablesMigratesLegacyTables(t *testing.T) {
	t.Setenv("ORIGIN_BLUEPRINT_CONFIG_PATH", filepath.Join(t.TempDir(), "config.json"))
	app := NewApp()
	workspace := t.TempDir()
	configs := filepath.Join(workspace, "configs")
	if err := os.MkdirAll(configs, 0755); err != nil {
		t.Fatal(err)
	}
	writeTestCSV(t, filepath.Join(configs, "skills.csv"), []string{
		"id,名称",
		"1001,火球术",
	})
	writeTestCSV(t, filepath.Join(configs, "items.csv"), []string{
		"id,名称",
		"2,剑",
	})
	tables, err := app.LoadConfigTables(workspace, ConfigTablesSettings{
		Tables: []legacyConfigTableRule{
			{Key: "skills", File: "skills.csv"},
			{Key: "items"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(tables) != 2 {
		t.Fatalf("tables = %#v", tables)
	}
	for _, table := range tables {
		if table.Name != table.Key || len(table.Entries) != 1 {
			t.Fatalf("migrated dataset = %#v", table)
		}
	}
}

// 来源文件找不到时数据集保留（missing 标记），恢复目录后自动生效。
func TestLoadConfigTablesKeepsMissingDataset(t *testing.T) {
	t.Setenv("ORIGIN_BLUEPRINT_CONFIG_PATH", filepath.Join(t.TempDir(), "config.json"))
	app := NewApp()
	workspace := t.TempDir()
	configs := filepath.Join(workspace, "configs")
	if err := os.MkdirAll(configs, 0755); err != nil {
		t.Fatal(err)
	}
	writeTestCSV(t, filepath.Join(configs, "skills.csv"), []string{
		"id,名称",
		"1001,火球术",
	})
	tables, err := app.LoadConfigTables(workspace, ConfigTablesSettings{
		Datasets: []ConfigTableDataset{
			{Key: "gone", Name: "已移走的表", File: "gone.csv"},
			{Key: "skills", Name: "skills", File: "skills.csv"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(tables) != 2 {
		t.Fatalf("tables = %#v", tables)
	}
	if !tables[0].Missing || tables[0].Entries != nil || !strings.Contains(tables[0].Warning, "未找到来源文件") {
		t.Fatalf("missing dataset = %#v", tables[0])
	}
	if tables[1].Missing || len(tables[1].Entries) != 1 {
		t.Fatalf("present dataset = %#v", tables[1])
	}
}

func TestLoadConfigTablesCarriesFilterExtraColumn(t *testing.T) {
	t.Setenv("ORIGIN_BLUEPRINT_CONFIG_PATH", filepath.Join(t.TempDir(), "config.json"))
	app := NewApp()
	workspace := t.TempDir()
	configs := filepath.Join(workspace, "configs")
	if err := os.MkdirAll(configs, 0755); err != nil {
		t.Fatal(err)
	}
	writeTestCSV(t, filepath.Join(configs, "skills.csv"), []string{
		"id,名称,备注",
		"5001,烈焰,主动技能",
		"5002,寒冰,被动技能",
	})
	tables, err := app.LoadConfigTables(workspace, ConfigTablesSettings{
		Datasets: []ConfigTableDataset{{
			Key: "skills", Name: "skills", File: "skills.csv",
			Filter: &ConfigTableFilter{ExtraKeyword: "备注", ExtraMatchRow: 1},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(tables) != 1 || len(tables[0].Entries) != 2 {
		t.Fatalf("entries = %#v", tables[0].Entries)
	}
	if tables[0].Entries[0].Extra != "主动技能" || tables[0].Entries[1].Extra != "被动技能" {
		t.Fatalf("extra = %#v", tables[0].Entries)
	}
}

func TestLoadConfigTablesAppliesSavedFilter(t *testing.T) {
	t.Setenv("ORIGIN_BLUEPRINT_CONFIG_PATH", filepath.Join(t.TempDir(), "config.json"))
	app := NewApp()
	workspace := t.TempDir()
	configs := filepath.Join(workspace, "configs")
	if err := os.MkdirAll(configs, 0755); err != nil {
		t.Fatal(err)
	}
	writeTestCSV(t, filepath.Join(configs, "restrict.csv"), []string{
		"id,名称,EnumName",
		"0,无限制,BehaviorRestrictType_NoneRestrict",
		"1,禁止移动,BehaviorRestrictType_NoMove",
		"9,禁止飞行,BehaviorRestrictType_NoFly",
		"10,法术伤害免疫,ImmuneType_MagicDamage",
	})
	settings := ConfigTablesSettings{Datasets: []ConfigTableDataset{{
		Key: "restrict", Name: "restrict", File: "restrict.csv",
		Filter: &ConfigTableFilter{
			Mode:          "text",
			Column:        "custom",
			Text:          "BehaviorRestrictType_",
			ExtraMatchRow: 1,
			ExtraKeyword:  "EnumName",
		},
	}}}
	tables, err := app.LoadConfigTables(workspace, settings)
	if err != nil {
		t.Fatal(err)
	}
	if len(tables) != 1 {
		t.Fatalf("tables = %#v", tables)
	}
	if len(tables[0].Entries) != 3 || tables[0].Entries[2].ID != "9" {
		t.Fatalf("内存索引应只保留筛选命中的行：entries = %#v", tables[0].Entries)
	}
	for _, entry := range tables[0].Entries {
		if entry.ID == "10" {
			t.Fatalf("未命中筛选的行不应进入内存索引：entries = %#v", tables[0].Entries)
		}
	}
	// 管理界面的预览仍看全量数据，便于调整筛选条件。
	preview, err := app.PreviewConfigTable(workspace, settings, "restrict", "EnumName", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Entries) != 4 {
		t.Fatalf("预览应返回全量条目：entries = %#v", preview.Entries)
	}
}

func TestLoadConfigTablesAppliesSavedRangeFilter(t *testing.T) {
	t.Setenv("ORIGIN_BLUEPRINT_CONFIG_PATH", filepath.Join(t.TempDir(), "config.json"))
	app := NewApp()
	workspace := t.TempDir()
	configs := filepath.Join(workspace, "configs")
	if err := os.MkdirAll(configs, 0755); err != nil {
		t.Fatal(err)
	}
	writeTestCSV(t, filepath.Join(configs, "items.csv"), []string{
		"id,名称",
		"0,无",
		"5,中",
		"9,高",
		"10,超",
	})
	tables, err := app.LoadConfigTables(workspace, ConfigTablesSettings{
		Datasets: []ConfigTableDataset{{
			Key: "items", Name: "items", File: "items.csv",
			Filter: &ConfigTableFilter{Mode: "range", RangeMin: "0", RangeMax: "9"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(tables) != 1 || len(tables[0].Entries) != 3 {
		t.Fatalf("范围筛选应只保留 0-9：entries = %#v", tables[0].Entries)
	}
}

func TestLoadConfigTablesSkipsBrokenDirectoryInsteadOfFailing(t *testing.T) {
	t.Setenv("ORIGIN_BLUEPRINT_CONFIG_PATH", filepath.Join(t.TempDir(), "config.json"))
	app := NewApp()
	workspace := t.TempDir()
	configs := filepath.Join(workspace, "configs")
	if err := os.MkdirAll(configs, 0755); err != nil {
		t.Fatal(err)
	}
	writeTestCSV(t, filepath.Join(configs, "skills.csv"), []string{
		"id,名称",
		"100233,火球术",
	})
	// 一个坏目录（路径被普通文件占用，旧实现会让全部数据集加载失败）+ 一个正常目录。
	blocked := filepath.Join(workspace, "blocked-dir")
	if err := os.WriteFile(blocked, []byte("not a directory"), 0644); err != nil {
		t.Fatal(err)
	}
	tables, err := app.LoadConfigTables(workspace, ConfigTablesSettings{
		Directories: []string{"configs", "blocked-dir"},
		Datasets: []ConfigTableDataset{
			{Key: "skills", Name: "技能表", File: "skills.csv"},
			{Key: "missing", Name: "缺失表", File: "absent.csv"},
		},
	})
	if err != nil {
		t.Fatalf("broken directory must not fail the whole load: %v", err)
	}
	if len(tables) != 2 {
		t.Fatalf("tables = %#v", tables)
	}
	var skills, missing *ConfigTable
	for i := range tables {
		switch tables[i].Key {
		case "skills":
			skills = &tables[i]
		case "missing":
			missing = &tables[i]
		}
	}
	if skills == nil || len(skills.Entries) != 1 || skills.Warning != "" {
		t.Fatalf("skills table = %#v", skills)
	}
	if missing == nil || !missing.Missing {
		t.Fatalf("missing table = %#v", missing)
	}
	if !strings.Contains(missing.Warning, "未找到来源文件") || !strings.Contains(missing.Warning, "来源目录不可用") {
		t.Fatalf("missing table warning should keep file diagnosis and add skip diagnosis: %q", missing.Warning)
	}
}
