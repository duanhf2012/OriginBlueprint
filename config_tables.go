package main

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

// ConfigTableDefaults 是 configTables.defaults 的默认识别规则：
// id 列在第 idMatchRow 行里找包含 idKeyword 的单元格，名称列在第 nameMatchRow 行里找
// 包含 nameKeywords 的单元格（id 与名称可以位于不同的表头行）；
// 数据从两个识别行中较大者的下一行开始读取（跳过类型行、注释行等多行表头）。
type ConfigTableDefaults struct {
	IDMatchRow   int      `json:"idMatchRow"`
	NameMatchRow int      `json:"nameMatchRow"`
	IDKeyword    string   `json:"idKeyword"`
	NameKeywords []string `json:"nameKeywords"`
}

// ConfigTableDataset 是导入清单 configTables.datasets 的一项。
// 节点参数绑定的是**数据集**而不是物理表：Key 是稳定标识（nodes/*.json 与 .obpf 里的 ref
// 存的值，创建后不变），Name 是可随时修改的显示名；
// 一个物理表（file+sheet）可以派生多个数据集，例如同一张枚举定义表按“枚举名列包含的
// 关键字”筛选出不同枚举段，分别绑定到不同输入参数。
type ConfigTableDataset struct {
	Key          string             `json:"key"`
	Name         string             `json:"name"`
	File         string             `json:"file"`
	Sheet        string             `json:"sheet,omitempty"`
	IDMatchRow   int                `json:"idMatchRow,omitempty"`
	NameMatchRow int                `json:"nameMatchRow,omitempty"`
	IDKeyword    string             `json:"idKeyword,omitempty"`
	NameKeyword  string             `json:"nameKeyword,omitempty"`
	Filter       *ConfigTableFilter `json:"filter,omitempty"`
}

// legacyConfigTableRule 是旧版 configTables.tables 清单项（一项=一张表），
// 读取时由 normalizeConfigTablesSettings 自动迁移为同名数据集。
type legacyConfigTableRule struct {
	Key          string             `json:"key"`
	File         string             `json:"file"`
	Sheet        string             `json:"sheet,omitempty"`
	IDMatchRow   int                `json:"idMatchRow,omitempty"`
	NameMatchRow int                `json:"nameMatchRow,omitempty"`
	IDKeyword    string             `json:"idKeyword,omitempty"`
	NameKeyword  string             `json:"nameKeyword,omitempty"`
	Filter       *ConfigTableFilter `json:"filter,omitempty"`
}

// ConfigTableFilter 是数据集保存的筛选设置。
// 加载内存索引时：按 extraMatchRow/extraKeyword 把筛选列（如备注列/枚举名列）的值带进条目，
// 并按 mode/column/text 等条件过滤——只有筛选命中的行才会进入内存供引用控件使用。
type ConfigTableFilter struct {
	Mode          string `json:"mode,omitempty"`
	Column        string `json:"column,omitempty"`
	Text          string `json:"text,omitempty"`
	RangeMin      string `json:"rangeMin,omitempty"`
	RangeMax      string `json:"rangeMax,omitempty"`
	CompareOp     string `json:"compareOp,omitempty"`
	CompareValue  string `json:"compareValue,omitempty"`
	ExtraMatchRow int    `json:"extraMatchRow,omitempty"`
	ExtraKeyword  string `json:"extraKeyword,omitempty"`
}

func (filter *ConfigTableFilter) matches(entry ConfigTableEntry) bool {
	if filter == nil {
		return true
	}
	switch filter.Mode {
	case "range":
		min, minErr := strconv.ParseFloat(strings.TrimSpace(filter.RangeMin), 64)
		max, maxErr := strconv.ParseFloat(strings.TrimSpace(filter.RangeMax), 64)
		if minErr != nil || maxErr != nil {
			return true
		}
		id, err := strconv.ParseFloat(entry.ID, 64)
		return err == nil && id >= min && id <= max
	case "compare":
		threshold, err := strconv.ParseFloat(strings.TrimSpace(filter.CompareValue), 64)
		if err != nil {
			return true
		}
		id, idErr := strconv.ParseFloat(entry.ID, 64)
		if idErr != nil {
			return false
		}
		switch filter.CompareOp {
		case ">":
			return id > threshold
		case ">=":
			return id >= threshold
		case "<":
			return id < threshold
		case "<=":
			return id <= threshold
		}
		return true
	default:
		text := strings.ToLower(strings.TrimSpace(filter.Text))
		if text == "" {
			return true
		}
		column := filter.Column
		if column == "" {
			column = "all"
		}
		if column == "id" {
			return strings.Contains(strings.ToLower(entry.ID), text)
		}
		if column == "name" {
			return strings.Contains(strings.ToLower(entry.Name), text)
		}
		if column == "custom" {
			return strings.Contains(strings.ToLower(entry.Extra), text)
		}
		return strings.Contains(strings.ToLower(entry.ID), text) ||
			strings.Contains(strings.ToLower(entry.Name), text) ||
			strings.Contains(strings.ToLower(entry.Extra), text)
	}
}

func filterTableEntries(table *ConfigTable, filter *ConfigTableFilter) {
	if filter == nil || len(table.Entries) == 0 {
		return
	}
	kept := make([]ConfigTableEntry, 0, len(table.Entries))
	for _, entry := range table.Entries {
		if filter.matches(entry) {
			kept = append(kept, entry)
		}
	}
	table.Entries = kept
}

type ConfigTablesSettings struct {
	// Directories 是表元目录列表（相对工作区根目录或绝对路径），所有可导入的表只从这些目录获取。
	Directories []string `json:"directories"`
	Defaults    ConfigTableDefaults `json:"defaults"`
	// Datasets 是数据集清单：节点引用绑定数据集，一张物理表可派生多个数据集。
	Datasets []ConfigTableDataset `json:"datasets"`
	// Tables 是旧版导入清单（一项=一张表），读取时自动迁移为同名数据集。
	Tables []legacyConfigTableRule `json:"tables,omitempty"`
}

// normalizeConfigTablesSettings 把旧版 tables 清单迁移为数据集（key/name 沿用旧表键，
// 已有节点 ref 引用不受影响），并给缺失的显示名补默认值。
func normalizeConfigTablesSettings(settings ConfigTablesSettings) ConfigTablesSettings {
	if len(settings.Datasets) == 0 && len(settings.Tables) > 0 {
		for _, rule := range settings.Tables {
			settings.Datasets = append(settings.Datasets, ConfigTableDataset{
				Key:          rule.Key,
				Name:         rule.Key,
				File:         rule.File,
				Sheet:        rule.Sheet,
				IDMatchRow:   rule.IDMatchRow,
				NameMatchRow: rule.NameMatchRow,
				IDKeyword:    rule.IDKeyword,
				NameKeyword:  rule.NameKeyword,
				Filter:       rule.Filter,
			})
		}
	}
	settings.Tables = nil
	for index := range settings.Datasets {
		if strings.TrimSpace(settings.Datasets[index].Name) == "" {
			settings.Datasets[index].Name = settings.Datasets[index].Key
		}
	}
	return settings
}

type ConfigTableEntry struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Extra string `json:"extra,omitempty"`
}

type ConfigTable struct {
	Key        string             `json:"key"`
	Name       string             `json:"name,omitempty"`
	File       string             `json:"file"`
	Sheet      string             `json:"sheet,omitempty"`
	RowCount   int                `json:"rowCount"`
	IDColumn   string             `json:"idColumn"`
	NameColumn string             `json:"nameColumn"`
	Imported   bool               `json:"imported"`
	Missing    bool               `json:"missing,omitempty"`
	Warning    string             `json:"warning,omitempty"`
	Entries    []ConfigTableEntry `json:"entries"`
}

type resolvedConfigTableRule struct {
	idMatchRow   int
	nameMatchRow int
	dataStartRow int
	idKeyword    string
	nameKeywords []string
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func defaultConfigTableRule(defaults ConfigTableDefaults) resolvedConfigTableRule {
	idMatchRow := defaults.IDMatchRow
	if idMatchRow < 1 {
		idMatchRow = 1
	}
	nameMatchRow := defaults.NameMatchRow
	if nameMatchRow < 1 {
		nameMatchRow = idMatchRow
	}
	idKeyword := strings.TrimSpace(defaults.IDKeyword)
	if idKeyword == "" {
		idKeyword = "id"
	}
	nameKeywords := splitKeywordList(defaults.NameKeywords)
	if len(nameKeywords) == 0 {
		nameKeywords = []string{"name", "名称"}
	}
	return resolvedConfigTableRule{idMatchRow: idMatchRow, nameMatchRow: nameMatchRow, dataStartRow: maxInt(idMatchRow, nameMatchRow) + 1, idKeyword: idKeyword, nameKeywords: nameKeywords}
}

func splitKeywordList(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		for _, part := range strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == '，' }) {
			if trimmed := strings.TrimSpace(part); trimmed != "" {
				result = append(result, trimmed)
			}
		}
	}
	return result
}

func (rule resolvedConfigTableRule) overrideWith(custom ConfigTableDataset) resolvedConfigTableRule {
	if custom.IDMatchRow > 0 {
		rule.idMatchRow = custom.IDMatchRow
	}
	if custom.NameMatchRow > 0 {
		rule.nameMatchRow = custom.NameMatchRow
	}
	rule.dataStartRow = maxInt(rule.idMatchRow, rule.nameMatchRow) + 1
	if keyword := strings.TrimSpace(custom.IDKeyword); keyword != "" {
		rule.idKeyword = keyword
	}
	if keywords := splitKeywordList([]string{custom.NameKeyword}); len(keywords) > 0 {
		rule.nameKeywords = keywords
	}
	return rule
}

func containsAnyKeyword(cell string, keywords []string) bool {
	lower := strings.ToLower(cell)
	for _, keyword := range keywords {
		if strings.TrimSpace(keyword) != "" && strings.Contains(lower, strings.ToLower(strings.TrimSpace(keyword))) {
			return true
		}
	}
	return false
}

// resolveConfigColumns 分别在 id 识别行和名称识别行里按“包含关键字”定位列。
func resolveConfigColumns(rows [][]string, rule resolvedConfigTableRule) (idColumn, nameColumn int, warning string) {
	idColumn = findKeywordColumn(rows, rule.idMatchRow, rule.idKeyword)
	nameColumn = findAnyKeywordColumn(rows, rule.nameMatchRow, rule.nameKeywords)
	warning = ""
	if idColumn < 0 || nameColumn < 0 {
		missing := make([]string, 0, 2)
		if idColumn < 0 {
			missing = append(missing, fmt.Sprintf("id 列（表头第 %d 行未含 %q）", rule.idMatchRow, rule.idKeyword))
		}
		if nameColumn < 0 {
			missing = append(missing, fmt.Sprintf("名称列（表头第 %d 行未含 %v）", rule.nameMatchRow, rule.nameKeywords))
		}
		warning = fmt.Sprintf("未识别出 %s", strings.Join(missing, " 和 "))
	}
	return idColumn, nameColumn, warning
}

func findKeywordColumn(rows [][]string, matchRow int, keyword string) int {
	if matchRow-1 < 0 || matchRow-1 >= len(rows) {
		return -1
	}
	for column, cell := range rows[matchRow-1] {
		text := strings.TrimSpace(cell)
		if text != "" && strings.Contains(strings.ToLower(text), strings.ToLower(keyword)) {
			return column
		}
	}
	return -1
}

func findAnyKeywordColumn(rows [][]string, matchRow int, keywords []string) int {
	if matchRow-1 < 0 || matchRow-1 >= len(rows) {
		return -1
	}
	for column, cell := range rows[matchRow-1] {
		text := strings.TrimSpace(cell)
		if text != "" && containsAnyKeyword(text, keywords) {
			return column
		}
	}
	return -1
}

func configEntriesFromRows(rows [][]string, rule resolvedConfigTableRule, includeEntries bool, extra columnExtraRule) (ConfigTable, error) {
	idColumn, nameColumn, warning := resolveConfigColumns(rows, rule)
	table := ConfigTable{RowCount: len(rows), Warning: warning}
	if idColumn < 0 || nameColumn < 0 {
		return table, nil
	}
	table.IDColumn = configColumnLabel(idColumn)
	table.NameColumn = configColumnLabel(nameColumn)
	if !includeEntries {
		return table, nil
	}
	extraColumn := -1
	if extra.keyword != "" {
		matchRow := extra.matchRow
		if matchRow < 1 {
			matchRow = rule.idMatchRow
		}
		extraColumn = findKeywordColumn(rows, matchRow, extra.keyword)
	}
	for row := rule.dataStartRow - 1; row < len(rows); row++ {
		cells := rows[row]
		id := configCell(cells, idColumn)
		if !looksLikeDataIdCell(id) {
			continue
		}
		entry := ConfigTableEntry{ID: id, Name: configCell(cells, nameColumn)}
		if extraColumn >= 0 {
			entry.Extra = configCell(cells, extraColumn)
		}
		table.Entries = append(table.Entries, entry)
	}
	return table, nil
}

// looksLikeDataIdCell 只保留 id 为数字的行：空或非数字（含类型行、注释行）自动跳过；
// 重复 id 不做去重和校验，原样保留显示。
func looksLikeDataIdCell(id string) bool {
	if id == "" {
		return false
	}
	_, err := strconv.ParseFloat(id, 64)
	return err == nil
}

func configCell(cells []string, column int) string {
	if column < 0 || column >= len(cells) {
		return ""
	}
	return strings.TrimSpace(cells[column])
}

func configColumnLabel(column int) string {
	if column < 0 {
		return ""
	}
	label, err := excelize.ColumnNumberToName(column + 1)
	if err != nil {
		return strconv.Itoa(column + 1)
	}
	return label
}

type columnExtraRule struct {
	keyword  string
	matchRow int
}

// mergeExtraRule：显式传入的预览筛选列优先；否则沿用数据集里保存的筛选列。
func mergeExtraRule(saved columnExtraRule, override columnExtraRule) columnExtraRule {
	if override.keyword != "" {
		return override
	}
	return saved
}

// configTableFileData 缓存单个配置文件的内容：csv 直接存行，xlsx 存各 sheet 的行。
// 同一文件派生的多个数据集只解析一次文件。
type configTableFileData struct {
	path    string
	isXlsx  bool
	csvRows [][]string
	sheets  []configTableSheetRows
}

type configTableSheetRows struct {
	name string
	rows [][]string
}

func loadConfigTableFile(path string) (*configTableFileData, error) {
	if strings.EqualFold(filepath.Ext(path), ".csv") {
		rows, err := readCSVRows(path)
		if err != nil {
			return nil, err
		}
		return &configTableFileData{path: path, csvRows: rows}, nil
	}
	workbook, err := excelize.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("open xlsx: %w", err)
	}
	defer workbook.Close()
	data := &configTableFileData{path: path, isXlsx: true}
	for _, sheet := range workbook.GetSheetList() {
		rows, err := workbook.GetRows(sheet)
		if err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			continue
		}
		data.sheets = append(data.sheets, configTableSheetRows{name: sheet, rows: rows})
	}
	if len(data.sheets) == 0 {
		return nil, errors.New("xlsx has no data sheets")
	}
	return data, nil
}

// rowsFor 返回文件里指定 sheet 的行；xlsx 的 sheet 留空时取第一个有数据的 sheet。
func (data *configTableFileData) rowsFor(sheet string) (rows [][]string, resolved string, ok bool) {
	if !data.isXlsx {
		if strings.TrimSpace(sheet) != "" {
			return nil, "", false
		}
		return data.csvRows, "", true
	}
	trimmed := strings.TrimSpace(sheet)
	if trimmed == "" {
		first := data.sheets[0]
		return first.rows, first.name, true
	}
	for _, item := range data.sheets {
		if item.name == trimmed {
			return item.rows, item.name, true
		}
	}
	return nil, "", false
}

// configFileIndex 按文件名（base name）索引表元目录里的配置文件；
// 多个目录存在同名文件时，先配置的目录优先。
type configFileIndex struct {
	byBase map[string]string
}

func buildConfigFileIndex(workspaceRoot string, directories []string) (*configFileIndex, error) {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	if workspaceRoot == "" {
		return nil, errors.New("loading config tables requires an open workspace")
	}
	absoluteRoot, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace %q: %w", workspaceRoot, err)
	}
	if len(directories) == 0 {
		directories = []string{"configs"}
	}
	index := &configFileIndex{byBase: map[string]string{}}
	for _, directory := range directories {
		directory = strings.TrimSpace(directory)
		if directory == "" {
			continue
		}
		target := directory
		if !filepath.IsAbs(target) {
			target = filepath.Join(absoluteRoot, filepath.FromSlash(directory))
		}
		files, err := sortedConfigTableFiles(target)
		if err != nil {
			return nil, err
		}
		for _, file := range files {
			base := filepath.Base(file)
			if _, exists := index.byBase[base]; !exists {
				index.byBase[base] = file
			}
		}
	}
	return index, nil
}

func datasetRuleFor(defaults ConfigTableDefaults, dataset ConfigTableDataset) resolvedConfigTableRule {
	return defaultConfigTableRule(defaults).overrideWith(dataset)
}

// lookupDatasetFile 定位数据集的来源文件；旧版清单可能只存表键（文件名去扩展名），
// 此时按“键+.csv / 键+.xlsx”回退匹配。
func lookupDatasetFile(index *configFileIndex, dataset ConfigTableDataset) (string, bool) {
	if file := strings.TrimSpace(dataset.File); file != "" {
		path, ok := index.byBase[file]
		return path, ok
	}
	key := strings.TrimSpace(dataset.Key)
	for _, candidate := range []string{key + ".csv", key + ".xlsx"} {
		if path, ok := index.byBase[candidate]; ok {
			return path, true
		}
	}
	return "", false
}

func datasetExtraRule(dataset ConfigTableDataset) columnExtraRule {
	if dataset.Filter == nil {
		return columnExtraRule{}
	}
	return columnExtraRule{keyword: strings.TrimSpace(dataset.Filter.ExtraKeyword), matchRow: dataset.Filter.ExtraMatchRow}
}

// LoadConfigTables 按 configTables.datasets 清单加载数据集：每个数据集按自己的识别规则解析
// 来源表并应用筛选，建立 id→名称索引供引用选择控件使用。
// 来源文件/sheet 未找到时数据集保留（missing 标记、不加载条目），恢复目录后自动生效。
func (a *App) LoadConfigTables(workspaceRoot string, settings ConfigTablesSettings) ([]ConfigTable, error) {
	settings = normalizeConfigTablesSettings(settings)
	index, err := buildConfigFileIndex(workspaceRoot, settings.Directories)
	if err != nil {
		return nil, err
	}
	cache := map[string]*configTableFileData{}
	result := make([]ConfigTable, 0, len(settings.Datasets))
	for _, dataset := range settings.Datasets {
		if strings.TrimSpace(dataset.Key) == "" {
			continue
		}
		result = append(result, loadConfigTableDataset(index, cache, settings.Defaults, dataset))
	}
	return result, nil
}

func loadConfigTableDataset(index *configFileIndex, cache map[string]*configTableFileData, defaults ConfigTableDefaults, dataset ConfigTableDataset) ConfigTable {
	path, found := lookupDatasetFile(index, dataset)
	if !found {
		return ConfigTable{
			Key: dataset.Key, Name: dataset.Name, File: dataset.File, Sheet: dataset.Sheet,
			Imported: true, Missing: true,
			Warning: fmt.Sprintf("未找到来源文件 %q（配置已保留，恢复目录后自动生效）", dataset.File),
		}
	}
	data := cache[path]
	if data == nil {
		loaded, err := loadConfigTableFile(path)
		if err != nil {
			return ConfigTable{
				Key: dataset.Key, Name: dataset.Name, File: dataset.File, Sheet: dataset.Sheet,
				Imported: true, Missing: true, Warning: fmt.Sprintf("读取来源文件失败：%v", err),
			}
		}
		data = loaded
		cache[path] = data
	}
	rows, resolvedSheet, ok := data.rowsFor(dataset.Sheet)
	if !ok {
		return ConfigTable{
			Key: dataset.Key, Name: dataset.Name, File: dataset.File, Sheet: dataset.Sheet,
			Imported: true, Missing: true, Warning: fmt.Sprintf("来源文件里没有 sheet %q", dataset.Sheet),
		}
	}
	table, err := configEntriesFromRows(rows, datasetRuleFor(defaults, dataset), true, datasetExtraRule(dataset))
	if err != nil {
		return ConfigTable{
			Key: dataset.Key, Name: dataset.Name, File: dataset.File, Sheet: dataset.Sheet,
			Imported: true, Missing: true, Warning: err.Error(),
		}
	}
	table.Key = dataset.Key
	table.Name = dataset.Name
	table.File = filepath.Base(path)
	table.Sheet = resolvedSheet
	table.Imported = true
	filterTableEntries(&table, dataset.Filter)
	return table
}

// ScanConfigTables 扫描表元目录列出**全部可导入的物理表**（按默认识别规则给出识别预览，
// 不加载条目），供“批量导入”界面勾选；imported 表示该表已派生至少一个数据集。
func (a *App) ScanConfigTables(workspaceRoot string, settings ConfigTablesSettings) ([]ConfigTable, error) {
	settings = normalizeConfigTablesSettings(settings)
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	if workspaceRoot == "" {
		return nil, errors.New("loading config tables requires an open workspace")
	}
	absoluteRoot, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace %q: %w", workspaceRoot, err)
	}
	directories := settings.Directories
	if len(directories) == 0 {
		directories = []string{"configs"}
	}
	seen := map[string]bool{}
	tables := make([]ConfigTable, 0, 8)
	for _, directory := range directories {
		directory = strings.TrimSpace(directory)
		if directory == "" {
			continue
		}
		target := directory
		if !filepath.IsAbs(target) {
			target = filepath.Join(absoluteRoot, filepath.FromSlash(directory))
		}
		files, err := sortedConfigTableFiles(target)
		if err != nil {
			return nil, err
		}
		for _, file := range files {
			stem := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
			if !strings.EqualFold(filepath.Ext(file), ".csv") {
				data, err := loadConfigTableFile(file)
				if err != nil {
					tables = append(tables, ConfigTable{Key: stem, File: filepath.Base(file), Warning: err.Error()})
					continue
				}
				for _, item := range data.sheets {
					key := stem
					if len(data.sheets) > 1 || !strings.EqualFold(item.name, stem) {
						key = fmt.Sprintf("%s/%s", stem, item.name)
					}
					table, _ := configEntriesFromRows(item.rows, defaultConfigTableRule(settings.Defaults), false, columnExtraRule{})
					table.Key = key
					table.File = filepath.Base(file)
					table.Sheet = item.name
					table.Imported = datasetCountForFile(settings, filepath.Base(file), item.name) > 0
					tables = append(tables, table)
				}
				continue
			}
			key := stem
			if seen[key] {
				key = fmt.Sprintf("%s/%s", stem, filepath.Base(file))
			}
			seen[key] = true
			rows, err := readCSVRows(file)
			if err != nil {
				tables = append(tables, ConfigTable{Key: key, File: filepath.Base(file), Warning: err.Error()})
				continue
			}
			table, _ := configEntriesFromRows(rows, defaultConfigTableRule(settings.Defaults), false, columnExtraRule{})
			table.Key = key
			table.File = filepath.Base(file)
			table.Imported = datasetCountForFile(settings, filepath.Base(file), "") > 0
			tables = append(tables, table)
		}
	}
	sort.Slice(tables, func(i, j int) bool { return tables[i].Key < tables[j].Key })
	return tables, nil
}

func datasetCountForFile(settings ConfigTablesSettings, file, sheet string) int {
	count := 0
	for _, dataset := range settings.Datasets {
		if dataset.File == "" || dataset.File != file {
			continue
		}
		if dataset.Sheet == "" || dataset.Sheet == sheet {
			count++
		}
	}
	return count
}

// PreviewConfigTable 按数据集（含草稿）解析来源表并返回**全量**条目（不应用筛选），
// 供“数据集管理”界面预览与调整筛选。extraKeyword/extraMatchRow 可显式指定筛选列，
// 未指定时沿用数据集保存的筛选列。
func (a *App) PreviewConfigTable(workspaceRoot string, settings ConfigTablesSettings, key, extraKeyword string, extraMatchRow int) (ConfigTable, error) {
	settings = normalizeConfigTablesSettings(settings)
	dataset := ConfigTableDataset{}
	found := false
	for _, item := range settings.Datasets {
		if item.Key == strings.TrimSpace(key) {
			dataset, found = item, true
			break
		}
	}
	if !found {
		return ConfigTable{}, fmt.Errorf("数据集 %q 不存在", strings.TrimSpace(key))
	}
	index, err := buildConfigFileIndex(workspaceRoot, settings.Directories)
	if err != nil {
		return ConfigTable{}, err
	}
	path, ok := lookupDatasetFile(index, dataset)
	if !ok {
		return ConfigTable{}, fmt.Errorf("未找到来源文件 %q", dataset.File)
	}
	data, err := loadConfigTableFile(path)
	if err != nil {
		return ConfigTable{}, err
	}
	rows, resolvedSheet, ok := data.rowsFor(dataset.Sheet)
	if !ok {
		return ConfigTable{}, fmt.Errorf("来源文件里没有 sheet %q", dataset.Sheet)
	}
	extra := mergeExtraRule(datasetExtraRule(dataset), columnExtraRule{keyword: strings.TrimSpace(extraKeyword), matchRow: extraMatchRow})
	table, err := configEntriesFromRows(rows, datasetRuleFor(settings.Defaults, dataset), true, extra)
	if err != nil {
		return ConfigTable{}, err
	}
	table.Key = dataset.Key
	table.Name = dataset.Name
	table.File = filepath.Base(path)
	table.Sheet = resolvedSheet
	table.Imported = true
	return table, nil
}

func sortedConfigTableFiles(directory string) ([]string, error) {
	if info, err := os.Stat(directory); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	} else if !info.IsDir() {
		return nil, fmt.Errorf("config table directory %q is not a directory", directory)
	}
	var files []string
	err := filepath.WalkDir(directory, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			if path != directory && strings.HasPrefix(entry.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext == ".xlsx" || ext == ".csv" {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func readCSVRows(path string) ([][]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	reader := csv.NewReader(strings.NewReader(string(data)))
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = -1
	return reader.ReadAll()
}
