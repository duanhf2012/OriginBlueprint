//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

const obprojProgID = "OriginBlueprint.obproj"

// registerObprojFileAssociation 把 .obproj 工程文件关联到当前程序（写入 HKCU，无需管理员权限），
// 之后在资源管理器双击 .obproj 即可启动本程序并打开对应工程。
// 开发模式（临时目录中的二进制）跳过注册，避免关联指向 go-build/wails 临时产物。
func registerObprojFileAssociation() error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	if resolved, resolveErr := filepath.EvalSymlinks(executable); resolveErr == nil {
		executable = resolved
	}
	if isUnderTempDirectory(executable) {
		return nil
	}
	classesRoot, err := registry.OpenKey(registry.CURRENT_USER, `Software\Classes`, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer classesRoot.Close()

	extensionKey, _, err := registry.CreateKey(classesRoot, ".obproj", registry.SET_VALUE)
	if err != nil {
		return err
	}
	if err := extensionKey.SetStringValue("", obprojProgID); err != nil {
		extensionKey.Close()
		return err
	}
	extensionKey.Close()

	progIDKey, _, err := registry.CreateKey(classesRoot, obprojProgID, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer progIDKey.Close()
	if err := progIDKey.SetStringValue("", "OriginBlueprint 工程文件"); err != nil {
		return err
	}
	iconKey, _, err := registry.CreateKey(progIDKey, "DefaultIcon", registry.SET_VALUE)
	if err != nil {
		return err
	}
	if err := iconKey.SetStringValue("", fmt.Sprintf(`"%s",0`, executable)); err != nil {
		iconKey.Close()
		return err
	}
	iconKey.Close()

	commandKey, _, err := registry.CreateKey(progIDKey, `shell\open\command`, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer commandKey.Close()
	return commandKey.SetStringValue("", fmt.Sprintf(`"%s" "%%1"`, executable))
}

func isUnderTempDirectory(path string) bool {
	temp := filepath.Clean(os.TempDir())
	compare := filepath.Clean(path)
	tempLower := strings.ToLower(temp)
	compareLower := strings.ToLower(compare)
	return compareLower == tempLower || strings.HasPrefix(compareLower, tempLower+string(os.PathSeparator))
}

// notifyAssociationChanged 通知资源管理器文件关联已更新，避免旧缓存。
func notifyAssociationChanged() {
	proc := syscall.NewLazyDLL("shell32.dll").NewProc("SHChangeNotify")
	const shcneAssocChanged = 0x08000000
	proc.Call(uintptr(shcneAssocChanged), 0, 0, 0)
}
