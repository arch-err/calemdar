package main

import "fmt"

func errNotImplemented(cmd string) error {
	return fmt.Errorf("%s: not yet implemented", cmd)
}
