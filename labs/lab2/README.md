# lab2 Raft

### Code

##### RequestVote RPC handler.

```go
func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	// Your code here (2A, 2B).
	rf.mu.Lock()
	reply.Term = rf.currentTerm
	if args.Term > rf.currentTerm {
		rf.currentTerm = args.Term
		rf.persist()
		rf.state = 0
		rf.DPrintf("[receive RV] %d (%d)",rf.me,args.CandidateID)

		//2B
		lastIdx := len(rf.log) - 1
		lastTerm := rf.log[lastIdx].Term
		//candidate’s log is at least as up-to-date as receiver’s log, grant vote
		if lastTerm < args.LastLogTerm || (lastTerm == args.LastLogTerm && lastIdx <= args.LastLogIndex) {
			reply.VoteGranted = true
			rf.ResetElectionTimeout()
			rf.voteFor = args.CandidateID
			rf.DPrintf("%d vote for %d and reset e-timeout",rf.me, args.CandidateID)
		} else {
			rf.DPrintf("%d not vote for %d: non-up-to-date",rf.me, args.CandidateID)
		}
	}else {
		rf.DPrintf("[%d reject low-term RV from %d]",rf.me, args.CandidateID)
	}
	rf.mu.Unlock()
}
```

##### AppendEntry RPC handler

```go
func (rf *Raft) AppendEntry(args *AppendEntryArgs, reply *AppendEntryReply) {
	//2A 2B
	rf.mu.Lock()
	reply.Term = rf.currentTerm
	if args.Term >= rf.currentTerm {
		rf.currentTerm = args.Term
		rf.persist()
		rf.state = 0
		rf.ResetElectionTimeout()
		rf.DPrintf("[receive AE]%d (%d)",rf.me,args.LeaderId)

		if len(rf.log) <= args.PrevLogIndex || rf.log[args.PrevLogIndex].Term != args.PrevLogTerm {
			if len(rf.log) > args.PrevLogIndex {
				jt := args.PrevLogIndex
				for rf.log[jt].Term == rf.log[args.PrevLogIndex].Term {
					jt--
				}
				jt++
				reply.JumpTo = jt
				if reply.JumpTo <= 0 {
					rf.DPrintf("ERROR,jt <= 0! %d", jt)
				}
			} else {
				reply.JumpTo = len(rf.log)
				if reply.JumpTo <= 0 {
					rf.DPrintf("ERROR,len(log) <= 0! %d", len(rf.log))
				}
			}
			rf.DPrintf("%d preUnMatch, jumpTo (%v)",rf.me, reply.JumpTo)
			rf.mu.Unlock()
			return
		}
		reply.Success = true
		rf.DPrintf("preMatch, processing")
		iter := args.PrevLogIndex + 1
		for i, entry := range args.Entries {
			if len(rf.log) <= iter || rf.log[iter].Term != entry.Term {
				rf.log = rf.log[:iter]
				rf.log = append(rf.log, args.Entries[i:]...)
				rf.DPrintf("%d append Logs",rf.me)
				rf.persist()
				break
			} else {
				iter++
			}
		}

		if args.LeaderCommit > rf.commitIndex {
			if args.LeaderCommit > len(rf.log)-1 {
				rf.DPrintf("ERROR leadercommit > rf.lastIdx")
			}
			rf.commitIndex = min(args.LeaderCommit, len(rf.log)-1)
			for rf.commitIndex > rf.lastApplied {
				rf.lastApplied++
				rf.applyCh <- ApplyMsg{true, rf.log[rf.lastApplied].Command, rf.lastApplied}
			}
			rf.DPrintf("%d commit index",rf.me)

		}
	}else {
		rf.DPrintf("%d reject low-term AE from %d",rf.me,args.LeaderId)
	}
	rf.mu.Unlock()
}
```

##### RequestVote

```go
func (rf *Raft) sendRequestVote(oldTerm int, lastIdx int, lastTerm int) {
	rf.mu.Lock()
	if rf.currentTerm != oldTerm || rf.state != 1{
		rf.mu.Unlock()
		return
	}
	rf.DPrintf("[%d send RVs]",rf.me)
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
			//added
			rf.mu.Lock()
			if rf.currentTerm != oldTerm || rf.state != 1{
				rf.mu.Unlock()
				mu.Lock()
				visited++
				cond.Broadcast()
				mu.Unlock()
				return
			}
			args := RequestVoteArgs{oldTerm, rf.me, lastIdx, lastTerm}
			reply := RequestVoteReply{0, false}
			rf.mu.Unlock()

			rf.peers[i].Call("Raft.RequestVote", &args, &reply)

			rf.mu.Lock()
			if reply.Term > rf.currentTerm {
				rf.currentTerm = reply.Term
				rf.persist()
				rf.state = 0
				rf.DPrintf("%d turns to follower in RV by %d",rf.me,i)
			}

			rf.mu.Unlock()
			mu.Lock()
			if reply.VoteGranted {
				get++
				rf.DPrintf("get++")
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
			rf.DPrintf("[becomes leader] %d/%d", get,visited)
		} else {
			rf.DPrintf("[lose election] %d/%d", get,visited)
		}
	} else {
		rf.DPrintf("election cond change:state %d != 1 or term %d != %d",rf.state,rf.currentTerm,oldTerm)
	}
	rf.mu.Unlock()
	mu.Unlock()
}
```

##### HeartBeat

```go
func (rf *Raft) sendHeartBeat( /*oldRf Raft*/ oldTerm int) {
	rf.mu.Lock()
	//added
	if rf.currentTerm != oldTerm || rf.state != 2{
		rf.mu.Unlock()
		return
	}
	rf.DPrintf("[%d send AEs]",rf.me)
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
			//added
			if rf.currentTerm != oldTerm || rf.state != 2{
				rf.mu.Unlock()
				mu.Lock()
				visited++
				cond.Broadcast()
				mu.Unlock()
				return
			}

			var s []LogEntry
			if len(rf.log) > rf.nextIndex[i] {
				s = rf.log[rf.nextIndex[i]:]
			}
			currTerm := rf.currentTerm
			args := AppendEntryArgs{oldTerm, rf.me, rf.nextIndex[i] - 1, rf.log[rf.nextIndex[i]-1].Term, rf.commitIndex, s}
			rf.mu.Unlock()
			reply := AppendEntryReply{0, false, 0}
			ok := false
			if currTerm == oldTerm {
				ok = rf.peers[i].Call("Raft.AppendEntry", &args, &reply) //so tricky
				if !ok{
					DPrintf("WARNING %d lost",i)
				}
			}

			rf.mu.Lock()
			if reply.Term > rf.currentTerm {
				rf.currentTerm = reply.Term
				rf.persist()
				rf.state = 0
				rf.DPrintf("%d turns to follower in AE by %d",rf.me, i)
			}

			flag := ok && rf.currentTerm == oldTerm && rf.state == 2 && rf.nextIndex[i]-1 == args.PrevLogIndex //condition hasn't changed
			if flag {
				if reply.Success {
					rf.nextIndex[i] = args.PrevLogIndex + len(args.Entries) + 1
					rf.matchIndex[i] = args.PrevLogIndex + len(args.Entries)
					rec = min(rf.matchIndex[i], rec)
					//rf.DPrintf("[%d inform leader%d preMatch] update nextIndex[%d]",rf.me, i,i)
				} else {
					//rf.nextIndex[i]--
					rf.nextIndex[i] = reply.JumpTo
					if rf.nextIndex[i] <= 0 {
						rf.DPrintf("ERROR!index out of range!%d", reply.JumpTo)
					}
					//rf.DPrintf("[%d inform leader%d preUnMatch] jumpTo %d",rf.me, i,reply.JumpTo)
				}
			}else {
				//rf.DPrintf("[AE cond changed]%d (%d)",rf.me, i)
			}
			rf.mu.Unlock()

			mu.Lock()
			if flag && reply.Success { //fixme
				applied++
				rf.DPrintf("applied++")
			}else{
				//rf.DPrintf("unApplied %v %v",flag,reply.Success)
				DPrintf("WARNING unapplied!!!check:%v %v==%v %v==2 %v==%v %v",ok,rf.currentTerm,oldTerm,rf.state,rf.nextIndex[i]-1,args.PrevLogIndex,reply.Success)
			}
			visited++
			cond.Broadcast()
			mu.Unlock()
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
				rf.DPrintf("[most committed, leader commit] %d/%d",applied,visited)
			}else{
				//rf.DPrintf("ERROR [double check FAIL]%d %d %d %d",rec,rf.commitIndex,rf.log[rec].Term,rf.currentTerm)todo
			}
		}else {
			rf.DPrintf("[non most commit,leader FAIL commit] %d/%d",applied,visited)
		}
	}else {
		rf.DPrintf("AE cond changed: state %d != 2 or term %d != %d",rf.state,rf.currentTerm,oldTerm)
	}
	rf.mu.Unlock()
	mu.Unlock()
}
```

### Gains and Reflections

1. The core of leader election lies in "term":the node with  largest term always gains leadership(2A).

2. After receiving the response, the term needs to be checked.If there is a term larger than "mine"--become a follower.

3. The tricky point is that if a server is disconnected, the Call function waits for the server to reply, during which time any subsequent code is not executed.（This means that if you are running a leadership election, the vote counting goroutine is blocked for that period of time, which looks like a deadlock.）

4. Be careful about assumptions across a drop and re-acquire of a lock. One place this can arise is when avoiding waiting with locks held. For example, this code to send vote RPCs is incorrect:

   ```go
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
   
5. When using a condition variable, do not return early. For example,when voting, if child goroutine find a peer whose term is higher than its own term, do not return immediately because the father goroutine is blocked and need child to awake it:

   ```go
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
   ```

6. The lock that is locked first must unlock later:

   ```go
   mu1.Lock()
   mu2.Lock()
       ...
   mu2.Unlock()
   mu1.Lock()
   
   //which means the following is incorrect(deadlock):
   mu2.Lock()
   mu1.Lock()
       ...
   mu2.Unlock()
   mu1.Lock()    
   ```

7. Upon receiving a heartbeat,they would truncate the follower’s log following `prevLogIndex`, and then append any entries included in the `AppendEntries` arguments. This is *also* not correct. We can once again turn to Figure 2:

   > *If* an existing entry conflicts with a new one (same index but different terms), delete the existing entry and all that follow it.

   The *if* here is crucial. If the follower has all the entries the leader sent, the follower **MUST NOT** truncate its log. Any elements *following* the entries sent by the leader **MUST** be kept. This is because we could be receiving an outdated `AppendEntries` RPC from the leader, and truncating the log would mean “taking back” entries that we may have already told the leader that we have in our log.

8. Livelocks:
   When your system livelocks, every node in your system is doing something, but collectively your nodes are in such a state that no progress is being made. This can happen fairly easily in Raft, especially if you do not follow Figure 2 religiously. One livelock scenario comes up especially often; no leader is being elected, or once a leader is elected, some other node starts an election, forcing the recently elected leader to abdicate immediately.
   
   Make sure you reset your election timer *exactly* when Figure 2 says you should. Specifically, you should *only* restart your election timer if a) you get an `AppendEntries` RPC from the *current* leader (i.e., if the term in the `AppendEntries` arguments is outdated, you should *not* reset your timer); b) you are starting an election; or c) you *grant* a vote to another peer.(Instead of receive a requestVote RPC,important!Because the RPC may come from a node whose log is out-dated.This may not cause a live lock,but will greatly reduce the performance of the system)
   
9. If a leader sends out an `AppendEntries` RPC, and it is rejected, but *not because of log inconsistency* (this can only happen if our term has passed), then you should immediately step down, and *not* update `nextIndex`. If you do, you could race with the resetting of `nextIndex` if you are re-elected immediately.

10. deadlock.

## Result

```shell
Done 2048/2048; 2048 ok, 0 failed
```

