//go:build !windows

package main

// 非 Windows 平台暂不注册文件关联。
func registerObprojFileAssociation() error { return nil }

func notifyAssociationChanged() {}
