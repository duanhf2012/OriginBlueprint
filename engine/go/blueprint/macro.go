package blueprint

import (
	"fmt"
	"strings"
)

// graphDocumentMacroRef 是引用图（.obp/.obpf）中记录的一条宏引用。
//
// 引用图只持久化稳定身份 macroId、外框几何与边界连线；宏名/路径等一切可从宏源推导的
// 信息不落盘。宏内容始终以 .obpm 源文件为准，编译期由加载层内联展开（热更时重新展开）。
type graphDocumentMacroRef struct {
	MacroID  string                       `json:"macroId"`
	Frame    graphDocumentMacroFrame      `json:"frame"`
	Boundary []graphDocumentMacroBoundary `json:"boundary,omitempty"`
}

// graphDocumentMacroFrame 是宏在引用图画布中的外框位置，仅编辑器使用，引擎不消费。
type graphDocumentMacroFrame struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// graphDocumentMacroBoundary 描述一条跨宏边界的连线：
// 宏镜像节点以宏文档 nodes 数组的下标（macroNodeIndex）标识，
// intoMacro=true 表示外部节点输出 → 宏节点输入，否则宏节点输出 → 外部节点输入。
type graphDocumentMacroBoundary struct {
	ExternalNode   string `json:"externalNode"`
	ExternalPort   string `json:"externalPort"`
	MacroNodeIndex int    `json:"macroNodeIndex"`
	MacroPort      string `json:"macroPort"`
	IntoMacro      bool   `json:"intoMacro"`
}

// macroResolver 由加载层提供：按 macroId 返回宏文档的独立副本；找不到时返回错误。
type macroResolver func(macroID string) (*graphDocument, error)

// expandMacroRefs 把文档中的宏引用内联展开为普通节点/连线/变量（就地修改）。
//
// 展开规则与编辑器 v1 复制语义对齐：
//   - 节点全部内联，ID 加 "macro:<refIndex>:<macroId>:" 前缀避免与宿主冲突；
//   - 与宿主同身份（同入口 class）的宏入口节点不重复创建，其下游连线改接宿主已有入口；
//   - 变量同名同类型复用宿主定义，缺失按源定义补建（ID 同前缀）；
//   - 宏内部连线随节点重映射重建；boundary 记录转成普通连线；
//   - 宏文档自身的宏引用递归展开（嵌套宏），按 macroId 链检测循环引用。
//
// 展开完成后 MacroRefs 清空，后续 graphDocumentToConfig 只见平铺文档。
func expandMacroRefs(document *graphDocument, resolve macroResolver, chain []string) error {
	if len(document.MacroRefs) == 0 {
		return nil
	}
	hostNodes := make(map[string]bool, len(document.Nodes))
	hostEntrances := make(map[string]string, len(document.Nodes))
	for _, node := range document.Nodes {
		hostNodes[node.ID] = true
		if class, ok := documentEntranceClass(node); ok {
			hostEntrances[class] = node.ID
		}
	}
	hostVariables := make(map[string]bool, len(document.Variables))
	variableByKey := make(map[string]string, len(document.Variables))
	for _, variable := range document.Variables {
		hostVariables[variable.ID] = true
		variableByKey[variableNameKey(variable)] = variable.ID
	}
	// 同一宏在同一图中只允许引用一次：入口按身份合并后，二次引用的内部逻辑会共享同一批
	// 入口节点，exec 出口将出现多个后继（编译期才报晦涩错误）。编辑器插入已防重，这里
	// 对手改文档直接给出明确诊断。
	seenMacroIDs := make(map[string]bool, len(document.MacroRefs))
	for refIndex, ref := range document.MacroRefs {
		macroID := strings.TrimSpace(ref.MacroID)
		if macroID == "" {
			return fmt.Errorf("macroRefs[%d]: macroId is empty", refIndex)
		}
		if seenMacroIDs[macroID] {
			return fmt.Errorf("macroRefs[%d]: macro %s is referenced multiple times; each macro may only be referenced once per graph", refIndex, macroID)
		}
		seenMacroIDs[macroID] = true
	}

	for refIndex, ref := range document.MacroRefs {
		macroID := strings.TrimSpace(ref.MacroID)
		if macroID == "" {
			return fmt.Errorf("macroRefs[%d]: macroId is empty", refIndex)
		}
		for _, visited := range chain {
			if visited == macroID {
				return fmt.Errorf("macro %s: cyclic macro reference", macroID)
			}
		}
		macroDocument, err := resolve(macroID)
		if err != nil {
			return fmt.Errorf("macroRefs[%d]: %w", refIndex, err)
		}
		if err := expandMacroRefs(macroDocument, resolve, append(chain, macroID)); err != nil {
			return fmt.Errorf("macro %s: %w", macroID, err)
		}
		prefix := fmt.Sprintf("macro:%d:%s:", refIndex, macroID)

		variableRemap := make(map[string]string, len(macroDocument.Variables))
		for _, variable := range macroDocument.Variables {
			if target, exists := variableByKey[variableNameKey(variable)]; exists {
				variableRemap[variable.ID] = target
				continue
			}
			merged := variable
			merged.ID = prefix + variable.ID
			if hostVariables[merged.ID] {
				return fmt.Errorf("macro %s: merged variable id %s conflicts with host", macroID, merged.ID)
			}
			document.Variables = append(document.Variables, merged)
			hostVariables[merged.ID] = true
			variableByKey[variableNameKey(merged)] = merged.ID
			variableRemap[variable.ID] = merged.ID
		}

		nodeRemap := make(map[string]string, len(macroDocument.Nodes))
		nodeByIndex := make([]string, len(macroDocument.Nodes))
		for index, node := range macroDocument.Nodes {
			targetID := prefix + node.ID
			if class, ok := documentEntranceClass(node); ok {
				if hostNodeID, exists := hostEntrances[class]; exists {
					// 同身份入口合并：宿主已有该入口时宏入口不重复创建。
					targetID = hostNodeID
				} else {
					hostEntrances[class] = targetID
				}
			}
			if !hostNodes[targetID] {
				merged := node
				merged.ID = targetID
				if merged.Properties.VariableID != "" {
					if remapped, exists := variableRemap[merged.Properties.VariableID]; exists {
						merged.Properties.VariableID = remapped
					}
				}
				document.Nodes = append(document.Nodes, merged)
				hostNodes[targetID] = true
			}
			nodeRemap[node.ID] = targetID
			nodeByIndex[index] = targetID
		}

		for _, connection := range macroDocument.Connections {
			source, ok := nodeRemap[connection.Source]
			if !ok {
				return fmt.Errorf("macro %s: connection source %s not found", macroID, connection.Source)
			}
			target, ok := nodeRemap[connection.Target]
			if !ok {
				return fmt.Errorf("macro %s: connection target %s not found", macroID, connection.Target)
			}
			if source == target {
				continue
			}
			document.Connections = append(document.Connections, graphDocumentConnection{
				Source:       source,
				SourceOutput: connection.SourceOutput,
				Target:       target,
				TargetInput:  connection.TargetInput,
			})
		}

		for boundaryIndex, boundary := range ref.Boundary {
			if boundary.MacroNodeIndex < 0 || boundary.MacroNodeIndex >= len(nodeByIndex) {
				return fmt.Errorf("macroRefs[%d].boundary[%d]: macro node index %d out of range", refIndex, boundaryIndex, boundary.MacroNodeIndex)
			}
			if !hostNodes[boundary.ExternalNode] {
				return fmt.Errorf("macroRefs[%d].boundary[%d]: external node %q not found", refIndex, boundaryIndex, boundary.ExternalNode)
			}
			macroNodeID := nodeByIndex[boundary.MacroNodeIndex]
			edge := graphDocumentConnection{
				Source:       boundary.ExternalNode,
				SourceOutput: boundary.ExternalPort,
				Target:       macroNodeID,
				TargetInput:  boundary.MacroPort,
			}
			if !boundary.IntoMacro {
				edge = graphDocumentConnection{
					Source:       macroNodeID,
					SourceOutput: boundary.MacroPort,
					Target:       boundary.ExternalNode,
					TargetInput:  boundary.ExternalPort,
				}
			}
			if edge.Source == edge.Target {
				continue
			}
			document.Connections = append(document.Connections, edge)
		}
	}
	document.MacroRefs = nil
	return nil
}

// documentEntranceClass 返回节点的入口 class；非入口节点 ok=false。
// 判定与编译器一致（parseEntranceClass），保证去重口径和编译期入口注册完全相同。
func documentEntranceClass(node graphDocumentNode) (string, bool) {
	class := strings.TrimSpace(node.Properties.LegacyClass)
	if class == "" {
		if spec, ok := documentNodeSpecs[node.TypeID]; ok {
			class = spec.class
		} else if spec, ok := externalDocumentNodeSpecs[node.TypeID]; ok {
			class = spec.class
		}
	}
	if class == "" {
		return "", false
	}
	if _, _, isEntrance := parseEntranceClass(class); !isEntrance {
		return "", false
	}
	return class, true
}

func variableNameKey(variable graphDocumentVariable) string {
	return strings.TrimSpace(variable.Name) + "\x00" + strings.TrimSpace(variable.Type)
}
