package raft

import "log"

// Debugging
const Debug = true

func DPrintf(format string, a ...interface{}) {
	if Debug {
		log.Printf(format, a...)
	}
}

func max(a int, b int) int {
	if a > b {
		return a
	} else {
		return b
	}
}
