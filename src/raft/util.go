package raft

import (
	"log"
	"time"
)

// Debugging
const Debug = 1
//original version
func DPrintf(format string, a ...interface{}) (n int, err error) {
	if Debug > 0 {
		log.Printf(format, a...)
	}
	return
}

func MyDPrintf(rf Raft, format string, a ...interface{}) (n int, err error) {
	if Debug > 0 {
		format = "%v: [peer %v (%v) at Term %v] " + format + "\n"
		a = append([]interface{}{time.Now().UnixNano()/1e6 - rf.allBegin, rf.me, rf.state, rf.currentTerm}, a...)
		log.Printf(format, a...)
	}
	return
}