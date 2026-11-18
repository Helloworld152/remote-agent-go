package main

import "os"

func userHome() string {
	home, _ := os.UserHomeDir()
	return home
}
