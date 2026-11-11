//go:build !windows

package main

import "errors"

func openLoginBrowser(string) error { return errors.New("browser authorization requires Windows") }
