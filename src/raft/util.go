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
		//format = "%v: [peer %v (%9s) at Term %v with Log %v (%v) tailterm (%v)] " + format + "\n"
		format = "%-5d: [peer %v %-9s at Term %-3d with Log %-3d %-3d tailterm %-3d] " + format + "\n"
		s := ""
		if rf.state == 0{
			s = "follower"
		}else if rf.state == 1{
			s = "candidate"
		}else{
			s = "leader"
		}
		a = append([]interface{}{time.Now().UnixNano()/1e6 - rf.allBegin, rf.me, s, rf.currentTerm,len(rf.log),rf.commitIndex,rf.log[len(rf.log) - 1].Term}, a...)
		log.Printf(format, a...)
	}
	return
}