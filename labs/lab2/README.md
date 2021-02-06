# lab2 Raft

## source code reading

- [x] raft.go
- [ ] src/labrpc

### Part 2A

> Implement Raft leader election and heartbeats (`AppendEntries` RPCs with no log entries). The goal for Part 2A is for a single leader to be elected, for the leader to remain the leader if there are no failures, and for a new leader to take over if the old leader fails or if packets to/from the old leader are lost. Run `go test -run 2A` to test your 2A code.

### Todolist

- [x] Raft struct
- [x] `RequestVoteArgs` and `RequestVoteReply` structs.
- [x] Make()
- [x] RequestVote()
- [x] `RequestVote()` RPC handler
- [x] define an `AppendEntries` RPC
- [x] `AppendEntries` RPC handler
- [x] getstate()

### Code

```
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

// RequestVote RPC handler.
func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	// Your code here (2A, 2B).
	//2A
	rf.mu.Lock()
	if rf.state == 1 {
		reply.Term = rf.currentTerm - 1
	} else {
		reply.Term = rf.currentTerm
	}
	if args.Term > rf.currentTerm { //fixme
		rf.currentTerm = args.Term
		rf.voteFor = args.CandidateID
		reply.VoteGranted = true
		rf.state = 0 //fixme for web partition then a re-election happened
		rf.ResetElectionTimeout()
	}
	rf.mu.Unlock()
}

// AppendEntry RPC handler
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
            //if there is a term larger than "mine",become a follower and return immediately(important!)
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
            //if there is a term larger than "mine",become a follower and return immediately(important!)
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
```

### Result

```shell
Test (2A): initial election ...
  ... Passed --   3.0  3   56   12898    0
Test (2A): election after network failure ...
  ... Passed --   7.9  3  174   31822    0
PASS
ok  	_/mnt/c/Users/litang/githubWorkSpace/MIT-6.824repo/src/raft	11.919s
```

### Gains and Reflections

1. The core of leader election lies in "term":the node with  largest term always gains leadership.

2. After receiving the response, the term needs to be checked.If there is a term larger than "mine"--become a follower and return.You can pass the test cases without doing that, but you have to be aware of that.

3. The tricky point is that if a server is disconnected, the Call function waits for the server to reply, during which time any subsequent code is not executed.（This means that if you are running a leadership election, the vote counting goroutine is blocked for that period of time, which looks like a deadlock.）

4. Be careful about assumptions across a drop and re-acquire of a lock. One place this can arise is when avoiding waiting with locks held. For example, this code to send vote RPCs is incorrect:

   ```
     rf.mu.Lock()
     rf.currentTerm += 1
     rf.state = Candidate
     for <each peer> {
       go func() {
         rf.mu.Lock()
         args.Term = rf.currentTerm
         rf.mu.Unlock()
         Call("Raft.RequestVote", &args, ...)
         // handle the reply...
       } ()
     }
     rf.mu.Unlock()
   ```

   The code sends each RPC in a separate goroutine. It's incorrect because args.Term may not be the same as the rf.currentTerm at which the surrounding code decided to become a Candidate. For example, multiple terms may come and go, and the peer may no longer be a candidate. One way to fix this is for the created goroutine to use a copy of rf.currentTerm made while the outer code holds the lock. Similarly, reply-handling code after the Call() must re-check all relevant assumptions after re-acquiring the lock; for example, it should check that rf.currentTerm hasn't changed since the decision to become a candidate.