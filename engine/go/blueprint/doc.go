// Package blueprint 实现 Origin 蓝图图的 Go 解析执行运行时。
//
// 已编译图结构在运行期保持只读，Create 出来的实例只保存变量、timer 等可变上下文。
package blueprint
