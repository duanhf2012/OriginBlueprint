//go:build windows

package main

import "golang.org/x/sys/windows"

func atomicReplaceFile(source, target string) error {
	return windows.Rename(source, target)
}
