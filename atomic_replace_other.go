//go:build !windows

package main

import "os"

func atomicReplaceFile(source, target string) error {
	return os.Rename(source, target)
}
