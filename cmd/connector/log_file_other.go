//go:build !linux

package main

import "os"

func logFileIdentity(info os.FileInfo) string { return info.Name() }
