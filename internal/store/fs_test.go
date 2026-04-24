package store

import "os"

func stat(p string) (os.FileInfo, error) { return os.Stat(p) }
