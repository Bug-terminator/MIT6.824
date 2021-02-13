package raft

import (
	"log"
	"time"
	//log "github.com/sirupsen/logrus"
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

func (rf *Raft)DPrintf( format string, a ...interface{}) (n int, err error) {
	if Debug > 0 {
		format = "%v: [peer %v (%v) at Term %v with Log %v (%v) tailterm (%v)] " + format + "\n"
		a = append([]interface{}{time.Now().UnixNano()/1e6 - rf.allBegin, rf.me, rf.state, rf.currentTerm,len(rf.log),rf.commitIndex,rf.log[len(rf.log) - 1].Term}, a...)
		log.Printf(format, a...)
	}
	return
}