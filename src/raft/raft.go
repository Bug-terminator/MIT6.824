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
	state                int   //0/1/2 for follower/candidate/leader
	heartbeatTimeout     int64 // leader heartbeat timeout
	electionTimeout      int64 // follower election timeout
	lastAE               int64 //the last time leader send AE
	lastVisited          int64 //the last time that followers were visited(receive a AE(follower) & RV(follower) & reply with a term larger than "me"(candidate))

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
	//2A
	rf.mu.Lock()
	if rf.state == 1 {
		reply.Term = rf.currentTerm - 1
	} else {
		reply.Term = rf.currentTerm
	}
	if args.Term > rf.currentTerm {
		rf.currentTerm = args.Term
		rf.voteFor = args.CandidateID
		reply.VoteGranted = true
		rf.state = 0 //for web partition then a re-election happened
		rf.ResetElectionTimeout()
	}
	rf.mu.Unlock()
}

//lab 2A AppendEntry struct and handler
type LogEntry struct {
	//todo
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
// append entry handler
func (rf *Raft) AppendEntry(args *AppendEntryArgs, reply *AppendEntryReply) {
	//2A
	rf.mu.Lock()
	reply.Term = rf.currentTerm
	if args.Term >= rf.currentTerm {
		//reply.Success = true//todo 2B
		rf.state = 0
		rf.currentTerm = args.Term
		rf.ResetElectionTimeout()
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

//
// the service using Raft (e.g. a k/v server) wants to start
// agreement on the next command to be appended to Raft's log. if this
// server isn't the leader, returns false. otherwise start the
// agreement and return immediately. there is no guarantee that this
// command will ever be committed to the Raft log, since the leader
// may fail or lose an election. even if the Raft instance has been killed,
// this function should return gracefully.
//
// the first return value is the index that the command will appear at
// if it's ever committed. the second return value is the current
// term. the third return value is true if this server believes it is
// the leader.
//
func (rf *Raft) Start(command interface{}) (int, int, bool) {
	index := -1
	term := -1
	isLeader := true

	// Your code here (2B).

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
	rf.heartbeatTimeout = 100
	rf.currentTerm = 0
	rf.state = 0
	rf.lastVisited = time.Now().UnixNano() / 1e6
	rf.electionTimeout = RandomTimeGenerator()
	go rf.HeartBeatMonitor()
	go rf.ReElectionMonitor()
	// initialize from state persisted before a crash
	rf.readPersist(persister.ReadRaftState())

	return rf
}

//request vote random timeout generator,range: [200,400)
func RandomTimeGenerator() int64 {
	rd := rand.New(rand.NewSource(time.Now().UnixNano())).Int63n(200) + 200
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

// check if election timeout have been reached,frequency: 1time/1ms
func (rf *Raft) ReElectionMonitor() {
	for !rf.killed() {
		timeNow := time.Now().UnixNano() / 1e6 //ms
		rf.mu.Lock()
		limit := rf.lastVisited + rf.electionTimeout
		var suceed bool = (limit < timeNow && rf.state == 0)
		rf.mu.Unlock()
		if suceed {
			rf.sendRequestVote()
		}
		time.Sleep(1 * time.Millisecond)
	}
}

//candidate send requestVote to all peers
func (rf *Raft) sendRequestVote() {
	//update related state
	rf.mu.Lock()
	rf.voteFor = rf.me
	rf.state = 1
	rf.currentTerm++
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
			args := RequestVoteArgs{rf.currentTerm, rf.me, 0, 0} //todo lab 2B need more
			reply := RequestVoteReply{0, false}
			rf.peers[i].Call("Raft.RequestVote", &args, &reply)
			mu.Lock()
			rf.mu.Lock()
			if reply.Term >= rf.currentTerm {
				rf.currentTerm = reply.Term
				rf.state = 0
				rf.ResetElectionTimeout()
				rf.mu.Unlock()
				cond.Signal()
				mu.Unlock()
				return
			}
			rf.mu.Unlock()
			if reply.VoteGranted {
				get++
			}
			visited++
			cond.Signal()
			mu.Unlock()
		}(i)
	}

	//count of votes
	mu.Lock()
	for get <= total/2 && visited < total {
		cond.Wait()
	}
	rf.mu.Lock()
	if get > total/2 {
		rf.state = 2 // become leader
		rf.ResetHeartBeatTimeout()
		rf.sendHeartBeat()
	} else {
		rf.state = 0
		rf.ResetElectionTimeout()
	}
	rf.mu.Unlock()
	mu.Unlock()
}

//check if heartBeat timeout have been reached
func (rf *Raft) HeartBeatMonitor() {
	for !rf.killed() {
		rf.mu.Lock()
		timeNow := time.Now().UnixNano() / 1e6 //ms
		limit := rf.lastAE + rf.heartbeatTimeout
		var succeed bool = (limit < timeNow && rf.state == 2)
		rf.mu.Unlock() // don't call time-consuming function with lock held
		if succeed {
			rf.ResetHeartBeatTimeout()
			rf.sendHeartBeat()
		}
		time.Sleep(1 * time.Millisecond)
	}
}

// leader send heartBeat to every follower
func (rf *Raft) sendHeartBeat() {
	for i, _ := range rf.peers {
		if i == rf.me {
			continue
		}
		go func(i int) {
			args := AppendEntryArgs{Term: rf.currentTerm, LeaderId: rf.me} //todo 2B need more
			reply := AppendEntryReply{0, false}
			rf.peers[i].Call("Raft.AppendEntry", &args, &reply)
			rf.mu.Lock()
			if reply.Term > rf.currentTerm {
				rf.currentTerm = reply.Term
				rf.state = 0
				rf.ResetElectionTimeout()
				rf.mu.Unlock()
				return
			}
			rf.mu.Unlock()
		}(i)
	}
}
