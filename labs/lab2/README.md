# lab2 Raft

## source code reading

- [x] raft.go
- [ ] src/labrpc

### Part 2A

> Implement Raft leader election and heartbeats (`AppendEntries` RPCs with no log entries). The goal for Part 2A is for a single leader to be elected, for the leader to remain the leader if there are no failures, and for a new leader to take over if the old leader fails or if packets to/from the old leader are lost. Run `go test -run 2A` to test your 2A code.

### Hint

- Follow the paper's Figure 2. At this point you care about sending and receiving RequestVote RPCs, the Rules for Servers that relate to elections, and the State related to leader election,
- Add the Figure 2 state for leader election to the `Raft` struct in `raft.go`. You'll also need to define a struct to hold information about each log entry.
- Fill in the `RequestVoteArgs` and `RequestVoteReply` structs. Modify `Make()` to create a background goroutine that will kick off leader election periodically by sending out `RequestVote` RPCs when it hasn't heard from another peer for a while. This way a peer will learn who is the leader, if there is already a leader, or become the leader itself. Implement the `RequestVote()` RPC handler so that servers will vote for one another.
- To implement heartbeats, define an `AppendEntries` RPC struct (though you may not need all the arguments yet), and have the leader send them out periodically. Write an `AppendEntries` RPC handler method that resets the election timeout so that other servers don't step forward as leaders when one has already been elected.
- Make sure the election timeouts in different peers don't always fire at the same time, or else all peers will vote only for themselves and no one will become the leader.
- The tester requires that the leader send heartbeat RPCs no more than ten times per second.
- The tester requires your Raft to elect a new leader within five seconds of the failure of the old leader (if a majority of peers can still communicate). Remember, however, that leader election may require multiple rounds in case of a split vote (which can happen if packets are lost or if candidates unluckily choose the same random backoff times). You must pick election timeouts (and thus heartbeat intervals) that are short enough that it's very likely that an election will complete in less than five seconds even if it requires multiple rounds.
- The paper's Section 5.2 mentions election timeouts in the range of 150 to 300 milliseconds. Such a range only makes sense if the leader sends heartbeats considerably more often than once per 150 milliseconds. Because the tester limits you to 10 heartbeats per second, you will have to use an election timeout larger than the paper's 150 to 300 milliseconds, but not too large, because then you may fail to elect a leader within five seconds.
- You may find Go's [rand](https://golang.org/pkg/math/rand/) useful.
- You'll need to write code that takes actions periodically or after delays in time. The easiest way to do this is to create a goroutine with a loop that calls [time.Sleep()](https://golang.org/pkg/time/#Sleep). Don't use Go's `time.Timer` or `time.Ticker`, which are difficult to use correctly.
- Read this advice about [locking](http://nil.csail.mit.edu/6.824/2020/labs/raft-locking.txt) and [structure](http://nil.csail.mit.edu/6.824/2020/labs/raft-structure.txt).
- If your code has trouble passing the tests, read the paper's Figure 2 again; the full logic for leader election is spread over multiple parts of the figure.
- Don't forget to implement `GetState()`.
- The tester calls your Raft's `rf.Kill()` when it is permanently shutting down an instance. You can check whether `Kill()` has been called using `rf.killed()`. You may want to do this in all loops, to avoid having dead Raft instances print confusing messages.
- A good way to debug your code is to insert print statements when a peer sends or receives a message, and collect the output in a file with `go test -run 2A > out`. Then, by studying the trace of messages in the `out` file, you can identify where your implementation deviates from the desired protocol. You might find `DPrintf` in `util.go` useful to turn printing on and off as you debug different problems.
- Go RPC sends only struct fields whose names start with capital letters. Sub-structures must also have capitalized field names (e.g. fields of log records in an array). The `labgob` package will warn you about this; don't ignore the warnings.
- Check your code with `go test -race`, and fix any races it reports.

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

```go
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