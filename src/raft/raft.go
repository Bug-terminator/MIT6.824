package raft

//
// this is an outline of the API that raft must expose to
// the service (or tester). see comments below for
// each of these functions for more details.
//
// rf = Make(...)
//   create a new Raft server.
// rf.Start(command interface{}) (index, term, isleader)
//   start agreement on a new log entry
// rf.GetState() (term, isLeader)
//   ask a Raft for its current term, and whether it thinks it is leader
// ApplyMsg
//   each time a new entry is committed to the log, each Raft peer
//   should send an ApplyMsg to the service (or tester)
//   in the same server.
//

import (
	"bytes"
	"labgob"
	"math"
	"math/rand"
	"sync"
	"time"
)
import "sync/atomic"
import "../labrpc"

// import "bytes"
// import "../labgob"

//
// as each Raft peer becomes aware that successive log entries are
// committed, the peer should send an ApplyMsg to the service (or
// tester) on the same server, via the applyCh passed to Make(). set
// CommandValid to true to indicate that the ApplyMsg contains a newly
// committed log entry.
//
// in Lab 3 you'll want to send other kinds of messages (e.g.,
// snapshots) on the applyCh; at that point you can add fields to
// ApplyMsg, but set CommandValid to false for these other uses.
//
type ApplyMsg struct {
	CommandValid bool
	Command      interface{}
	CommandIndex int
}

//
// A Go object implementing a single Raft peer.
//
type Raft struct {
	mu        sync.Mutex          // Lock to protect shared access to this peer's state
	peers     []*labrpc.ClientEnd // RPC end points of all peers
	persister *Persister          // Object to hold this peer's persisted state
	me        int                 // this peer's index into peers[]
	dead      int32               // set by Kill()

	// Your data here (2A, 2B, 2C).
	// 2A
	currentTerm, voteFor int   //term number & vote for who
	state                int32 //0/1/2 for follower/candidate/leader
	heartbeatTimeout     int64 // leader heartbeat timeout
	electionTimeout      int64 // follower election timeout
	lastAE               int64 //the last time leader send AE
	lastVisited          int64 //the last time that followers were visited(receive a AE(follower) & RV(follower) & reply with a term larger than "me"(candidate))
	allBegin             int64 // the very begining of the program
	//2B
	commitIndex int        // index of highest log entry known to be committed
	lastApplied int        // index of highest log entry applied to state machine
	nextIndex   []int      //for each server, index of the next log entry to send to that server
	matchIndex  []int      // for each server, index of highest log entry known to be replicated on server
	log         []LogEntry //log entries; each entry contains command for state machine, and term when entry was received by leader
	applyCh     chan ApplyMsg
	// Look at the paper's Figure 2 for a description of what
	// state a Raft server must maintain.

}

// return currentTerm and whether this server
// believes it is the leader.
func (rf *Raft) GetState() (int, bool) {

	var term int
	var isleader bool
	// Your code here (2A).
	rf.mu.Lock()
	term = rf.currentTerm
	isleader = (rf.state == 2)
	rf.mu.Unlock()
	return term, isleader
}

//
// save Raft's persistent state to stable storage,
// where it can later be retrieved after a crash and restart.
// see paper's Figure 2 for a description of what should be persistent.
//
func (rf *Raft) persist() {
	// Your code here (2C).
	// Example:
	// w := new(bytes.Buffer)
	// e := labgob.NewEncoder(w)
	// e.Encode(rf.xxx)
	// e.Encode(rf.yyy)
	// data := w.Bytes()
	// rf.persister.SaveRaftState(data)
	w := new(bytes.Buffer)
	e := labgob.NewEncoder(w)
	e.Encode(rf.currentTerm)
	e.Encode(rf.log)
	data := w.Bytes()
	rf.persister.SaveRaftState(data)
}

//
// restore previously persisted state.
//
func (rf *Raft) readPersist(data []byte) {
	if data == nil || len(data) < 1 { // bootstrap without any state?
		return
	}
	// Your code here (2C).
	// Example:
	// r := bytes.NewBuffer(data)
	// d := labgob.NewDecoder(r)
	// var xxx
	// var yyy
	// if d.Decode(&xxx) != nil ||
	//    d.Decode(&yyy) != nil {
	//   error...
	// } else {
	//   rf.xxx = xxx
	//   rf.yyy = yyy
	// }
	r := bytes.NewBuffer(data)
	d := labgob.NewDecoder(r)
	var currTerm int
	var log []LogEntry
	if d.Decode(&currTerm) != nil ||
		d.Decode(&log) != nil {
		DPrintf("decode error")
	} else {
		rf.currentTerm = currTerm
		rf.log = log
	}
}

//
// example RequestVote RPC arguments structure.
// field names must start with capital letters!
//
type RequestVoteArgs struct {
	// Your data here (2A, 2B).
	Term, CandidateID, LastLogIndex, LastLogTerm int
}

//
// example RequestVote RPC reply structure.
// field names must start with capital letters!
//
type RequestVoteReply struct {
	// Your data here (2A).
	Term        int
	VoteGranted bool
}

//
// example RequestVote RPC handler.
//
func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	// Your code here (2A, 2B).
	rf.mu.Lock()
	rf.DPrintf("receive RV")
	reply.Term = rf.currentTerm
	if args.Term > rf.currentTerm {
		rf.currentTerm = args.Term
		rf.state = 0
		//2B
		lastIdx := len(rf.log) - 1
		lastTerm := rf.log[lastIdx].Term
		//candidate’s log is at least as up-to-date as receiver’s log, grant vote
		if lastTerm < args.LastLogTerm || (lastTerm == args.LastLogTerm && lastIdx <= args.LastLogIndex) {
			reply.VoteGranted = true
			rf.ResetElectionTimeout()
			rf.voteFor = args.CandidateID
		}
	}
	rf.mu.Unlock()
}

//lab 2A AppendEntry struct and handler
type LogEntry struct {
	Command interface{}
	Term    int
}

type AppendEntryArgs struct {
	// Your data here (2A, 2B).
	Term, LeaderId, PrevLogIndex, PrevLogTerm, LeaderCommit int
	Entries                                                 []LogEntry
}

type AppendEntryReply struct {
	// Your data here (2A).
	Term    int
	Success bool
}

//AppendEntry handler
func (rf *Raft) AppendEntry(args *AppendEntryArgs, reply *AppendEntryReply) {
	//2A 2B
	rf.mu.Lock()
	rf.DPrintf("receive AE")
	reply.Term = rf.currentTerm
	if args.Term >= rf.currentTerm {
		rf.currentTerm = args.Term
		rf.state = 0
		rf.ResetElectionTimeout()

		if len(rf.log) <= args.PrevLogIndex || rf.log[args.PrevLogIndex].Term != args.PrevLogTerm {
			rf.mu.Unlock()
			return
		}
		reply.Success = true

		iter := args.PrevLogIndex + 1
		for i, entry := range args.Entries {
			if len(rf.log) <= iter || rf.log[iter].Term != entry.Term {
				rf.log = rf.log[:iter]
				rf.log = append(rf.log, args.Entries[i:]...)
				break
			} else {
				iter++
			}
		}

		if args.LeaderCommit > rf.commitIndex {
			rf.commitIndex = min(args.LeaderCommit, len(rf.log)-1)
			for rf.commitIndex > rf.lastApplied {
				rf.lastApplied++
				rf.applyCh <- ApplyMsg{true, rf.log[rf.lastApplied].Command, rf.lastApplied}
			}
		}
	}
	rf.mu.Unlock()
}

//
// example code to send a RequestVote RPC to a server.
// server is the index of the target server in rf.peers[].
// expects RPC arguments in args.
// fills in *reply with RPC reply, so caller should
// pass &reply.
// the types of the args and reply passed to Call() must be
// the same as the types of the arguments declared in the
// handler function (including whether they are pointers).
//
// The labrpc package simulates a lossy network, in which servers
// may be unreachable, and in which requests and replies may be lost.
// Call() sends a request and waits for a reply. If a reply arrives
// within a timeout interval, Call() returns true; otherwise
// Call() returns false. Thus Call() may not return for a while.
// A false return can be caused by a dead server, a live server that
// can't be reached, a lost request, or a lost reply.
//
// Call() is guaranteed to return (perhaps after a delay) *except* if the
// handler function on the server side does not return.  Thus there
// is no need to implement your own timeouts around Call().
//
// look at the comments in ../labrpc/labrpc.go for more details.
//
// if you're having trouble getting RPC to work, check that you've
// capitalized all field names in structs passed over RPC, and
// that the caller passes the address of the reply struct with &, not
// the struct itself.
//
func (rf *Raft) sendRequestVote1(server int, args *RequestVoteArgs, reply *RequestVoteReply) bool {
	ok := rf.peers[server].Call("Raft.RequestVote", args, reply)
	return ok
}
func min(a int, b int) int {
	if a > b {
		return b
	} else {
		return a
	}
}
func max(a int, b int) int {
	if a > b {
		return a
	} else {
		return b
	}
}

//
//the service using Raft (e.g. a k/v server) wants to start
//agreement on the next command to be appended to Raft's log. if this
//server isn't the leader, returns false. otherwise start the
//agreement and return immediately. there is no guarantee that this
//command will ever be committed to the Raft log, since the leader
//may fail or lose an election. even if the Raft instance has been killed,
//this function should return gracefully.
//
//the first return value is the index that the command will appear at
//if it's ever committed. the second return value is the current
//term. the third return value is true if this server believes it is
//the leader.

func (rf *Raft) Start(command interface{}) (int, int, bool) {
	index := -1
	term := -1
	isLeader := true

	// Your code here (2B)
	rf.mu.Lock()
	index = len(rf.log)
	term = rf.currentTerm
	isLeader = rf.state == 2

	if isLeader {
		rf.log = append(rf.log, LogEntry{command, term})
	}
	rf.mu.Unlock()
	return index, term, isLeader
}

//
// the tester doesn't halt goroutines created by Raft after each test,
// but it does call the Kill() method. your code can use killed() to
// check whether Kill() has been called. the use of atomic avoids the
// need for a lock.
//
// the issue is that long-running goroutines use memory and may chew
// up CPU time, perhaps causing later tests to fail and generating
// confusing debug output. any goroutine with a long-running loop
// should call killed() to check whether it should stop.
//
func (rf *Raft) Kill() {
	atomic.StoreInt32(&rf.dead, 1)
	// Your code here, if desired.
}

func (rf *Raft) killed() bool {
	z := atomic.LoadInt32(&rf.dead)
	return z == 1
}

//
// the service or tester wants to create a Raft server. the ports
// of all the Raft servers (including this one) are in peers[]. this
// server's port is peers[me]. all the servers' peers[] arrays
// have the same order. persister is a place for this server to
// save its persistent state, and also initially holds the most
// recent saved state, if any. applyCh is a channel on which the
// tester or service expects Raft to send ApplyMsg messages.
// Make() must return quickly, so it should start goroutines
// for any long-running work.
//
func Make(peers []*labrpc.ClientEnd, me int,
	persister *Persister, applyCh chan ApplyMsg) *Raft {
	rf := &Raft{}
	rf.peers = peers
	rf.persister = persister
	rf.me = me

	// Your initialization code here (2A, 2B, 2C).
	//2A
	rf.heartbeatTimeout = 100
	rf.currentTerm = 0
	rf.state = 0
	rf.ResetElectionTimeout()
	rf.allBegin = rf.lastVisited
	//2B
	rf.log = make([]LogEntry, 1)
	rf.log[0] = LogEntry{nil, 0} //dummy element
	rf.commitIndex = 0
	rf.lastApplied = 0
	rf.nextIndex = make([]int, len(rf.peers))
	rf.matchIndex = make([]int, len(rf.peers))
	rf.applyCh = applyCh
	go rf.HeartBeatMonitor()
	go rf.ReElectionMonitor()
	// initialize from state persisted before a crash
	rf.readPersist(persister.ReadRaftState())

	return rf
}

//request vote random timeout generator,range: [200,400)
func RandomTimeGenerator() int64 {
	rd := rand.New(rand.NewSource(time.Now().UnixNano())).Int63n(500) + 150
	return rd
}

//reset timeout
func (rf *Raft) ResetElectionTimeout() {
	rf.lastVisited = time.Now().UnixNano() / 1e6
	rf.electionTimeout = RandomTimeGenerator()
}

func (rf *Raft) ResetHeartBeatTimeout() {
	rf.lastAE = time.Now().UnixNano() / 1e6
}

// check if election timeout have been reached,frequency = 1 times/1 ms
func (rf *Raft) ReElectionMonitor() {
	i := 0
	for !rf.killed() {
		rf.mu.Lock()
		i++
		if i%500 == 0 { //debug
			rf.DPrintf("alive")
		}
		timeNow := time.Now().UnixNano() / 1e6 //ms
		limit := rf.lastVisited + rf.electionTimeout
		if limit <= timeNow && rf.state != 2 {
			oldTerm := rf.currentTerm + 1
			lastIdx := len(rf.log) - 1
			lastTerm := rf.log[lastIdx].Term
			rf.voteFor = rf.me
			rf.state = 1
			rf.currentTerm++
			rf.ResetElectionTimeout()
			go rf.sendRequestVote(oldTerm, lastIdx, lastTerm)
		}
		rf.mu.Unlock()
		time.Sleep(1 * time.Millisecond)
	}
	rf.mu.Lock()
	rf.DPrintf("dead")
	rf.mu.Unlock()
}

//candidate send requestVote to all peers
func (rf *Raft) sendRequestVote(oldTerm int, lastIdx int, lastTerm int) {
	rf.mu.Lock()
	rf.DPrintf("send RVs")
	rf.mu.Unlock()
	//concurrency request vote
	total := len(rf.peers)
	get, visited := 1, 1 //vote for itself
	mu := sync.Mutex{}
	cond := sync.NewCond(&mu)

	for i, _ := range rf.peers {
		if i == rf.me {
			continue
		}
		go func(i int) {
			args := RequestVoteArgs{oldTerm, rf.me, lastIdx, lastTerm}
			reply := RequestVoteReply{0, false}
			rf.peers[i].Call("Raft.RequestVote", &args, &reply)

			rf.mu.Lock()
			if reply.Term > rf.currentTerm {
				rf.currentTerm = reply.Term
				rf.state = 0
				//don't do this:
				//rf.mu.Unlock()
				//return
			}
			rf.mu.Unlock()
			mu.Lock()
			if reply.VoteGranted {
				get++
			}
			visited++
			cond.Broadcast()
			mu.Unlock()
		}(i)
	}

	mu.Lock()
	for get <= total/2 && visited < total {
		cond.Wait()
	}
	rf.mu.Lock()
	if rf.state == 1 && rf.currentTerm == oldTerm { //conditions haven't changed since it decide to be a candidate
		if get > total/2 {
			rf.state = 2
			for i, _ := range rf.nextIndex {
				rf.nextIndex[i] = lastIdx + 1
				rf.matchIndex[i] = 0
			}
			rf.ResetHeartBeatTimeout()
			go rf.sendHeartBeat(oldTerm)
		}
	}
	rf.mu.Unlock()
	mu.Unlock()
}

//check that rf.currentTerm hasn't changed since the decision to become a candidate.
//func (rf *Raft) NotChanged() bool {
//	//ct := atomic.LoadInt32(&rf.currentTerm)
//	rf.mu.Lock()
//	ret := (rf.state == 1)
//	rf.mu.Unlock()
//	return ret
//}

//check if heartBeat timeout have been reached
func (rf *Raft) HeartBeatMonitor() { //todo improve use for-select rewrite this function
	for !rf.killed() {
		rf.mu.Lock()
		timeNow := time.Now().UnixNano() / 1e6 //ms
		limit := rf.lastAE + rf.heartbeatTimeout
		if limit <= timeNow && rf.state == 2 {
			oldTerm := rf.currentTerm
			rf.ResetHeartBeatTimeout()
			go rf.sendHeartBeat(oldTerm) //must use goroutine to avoid live lock
		}
		rf.mu.Unlock()
		time.Sleep(1 * time.Millisecond)
	}

}

// leader send heartBeat to every follower
func (rf *Raft) sendHeartBeat( /*oldRf Raft*/ oldTerm int) {
	rf.mu.Lock()
	rf.DPrintf("send AEs")
	rf.mu.Unlock()
	total := len(rf.peers)
	rec := math.MaxInt32
	applied, visited := 1, 1
	mu := sync.Mutex{}
	cond := sync.NewCond(&mu)
	for i, _ := range rf.peers {
		if i == rf.me {
			continue
		}
		go func(i int) {
			rf.mu.Lock()
			s := []LogEntry{}
			if len(rf.log) > rf.nextIndex[i] {
				s = rf.log[rf.nextIndex[i]:]
			}
			currTerm := rf.currentTerm
			args := AppendEntryArgs{oldTerm, rf.me, rf.nextIndex[i] - 1, rf.log[rf.nextIndex[i]-1].Term, rf.commitIndex, s}
			rf.mu.Unlock()
			reply := AppendEntryReply{0, false}
			ok := false
			if currTerm == oldTerm {
				ok = rf.peers[i].Call("Raft.AppendEntry", &args, &reply) //so tricky
			}

			rf.mu.Lock()
			if reply.Term > rf.currentTerm {
				rf.currentTerm = reply.Term
				rf.state = 0
			}
			mu.Lock()
			if ok && rf.currentTerm == oldTerm && rf.state == 2 { //condition hasn't changed
				if reply.Success {
					applied++
					// update nextIndex and matchIndex  should use previous args' value !!!!
					//max is to prevent downgrading nextIndex from an out-dated reply
					rf.nextIndex[i] = max(args.PrevLogIndex+len(args.Entries)+1, rf.nextIndex[i])
					rf.matchIndex[i] = max(args.PrevLogIndex+len(args.Entries), rf.matchIndex[i])
					rec = min(rf.matchIndex[i], rec)
				} else {
					if args.PrevLogIndex == rf.nextIndex[i]-1 {
						rf.nextIndex[i]--
					}
				}
			}
			visited++
			cond.Broadcast()
			mu.Unlock()
			rf.mu.Unlock()
		}(i)
	}

	mu.Lock()
	for applied <= total/2 && visited < total {
		cond.Wait()
	}
	rf.mu.Lock()
	if rf.currentTerm == oldTerm && rf.state == 2 { //conditions haven't been changed since it send heartbeat
		if applied > total/2 {
			if rec > rf.commitIndex && rf.log[rec].Term == rf.currentTerm {
				rf.commitIndex = rec
				for rf.commitIndex > rf.lastApplied {
					rf.lastApplied++
					rf.applyCh <- ApplyMsg{true, rf.log[rf.lastApplied].Command, rf.lastApplied}
				}
			}
		}
	}
	rf.mu.Unlock()
	mu.Unlock()
}
