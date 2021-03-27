## Lab2 Raft

本文将不再提供lab的全部代码，只提供关键部分的伪代码，因为raft的整体流程在论文和视频里已经非常清楚了，我做完的lab2也有700多行（想看源代码可以去src/raft/raft.go)，就不再结合代码把流程复述一次了。

整个lab围绕Raft论文中的图2展开，我们的任务就是将下图转化为代码：

![image-20210327193141491](https://github.com/Bug-terminator/MIT6.824/blob/master/labs/lab2/Figure2.png)

按照上图，定义Raft数据结构如下：

```go
type Raft struct {
	mu        sync.Mutex          // Lock to protect shared access to this peer's state
	peers     []*labrpc.ClientEnd // RPC end points of all peers
	persister *Persister          // Object to hold this peer's persisted state
	me        int                 // this peer's index into peers[]
	dead      int32               // set by Kill()

	// 2A
	currentTerm, voteFor int   //term number & vote for who
	state                int32 //0/1/2 for follower/candidate/leader
	heartbeatTimeout     int64 // leader heartbeat timeout
	electionTimeout      int64 // follower election timeout
	lastAE               int64 //the last time leader send AE
	lastVisited          int64 //the last time that followers were visited
	allBegin             int64 // the very begining of the program
	//2B
	commitIndex int        // index of highest log entry known to be committed
	lastApplied int        // index of highest log entry applied to state machine
	nextIndex   []int      //for each server, index of the next log entry to send to that server
	matchIndex  []int      // for each server, index of highest log entry known to be replicated on server
	log         []LogEntry //log entries; each entry contains command for state machine, and term when entry was received by leader
	applyCh     chan ApplyMsg
}
```

本次lab总共分为三个部分，对应Raft论文中的第五节：

1. 领导人选举：这部分的目标是选举一位leader，如果leader失效，则需要选出一位新的leader。
2. 日志复制：这部分需要实现将leader中的日志复制到follower中去。
3. 日志提交：如果基于Raft的服务器重新启动，则应从中断的位置恢复服务。这就要求Raft保持”持久状态“（对应上图中的Persistent State），使其在重启后仍然有效。这部分就是要实现将”持久状态“永久保存至磁盘中。

本文主要是我总结的在实现lab2时需要关注的几个要点：

1. 状态机
2. 原子钟的设计
3. 投票系统的设计
4. 重置原子钟的时机
5. 日志复制的优化

### 状态机

![image-20210327193734855](https://github.com/Bug-terminator/MIT6.824/blob/master/labs/lab2/%E7%8A%B6%E6%80%81%E6%9C%BA.png)

每一个结点分为3种状态:follower,candidate和leader。Follower只响应来自其他服务器的RPC,如果一个follower在规定时间内没有收到RPC，它就成为一个candidate并开始选举。获得整个集群多数选票的候选人成为新的leader。Leader通常会一直工作到失效。同一个term里，状态机只可能成为candidate和leader一次，所以每次做出行为前都要检查状态和term，尤其是接受到rpc的result之后，因为此时状态可能已经改变了，如果不检查一可能产生错误的结果，二是不能及时释放锁或退出协程，会浪费cpu资源。

### 原子钟

如何实现在规定时间内没有收到RPC就触发选举的函数？我的思路是每隔一个很小的时间片，例如1ms，检查一下当前时间是否大于结点上次收到RPC的时间+随机产生的选举时间（500~650ms)。

```go
func (rf *Raft) ReElectionMonitor() {
	for !rf.killed() {
		rf.mu.Lock()
		timeNow := time.Now().UnixNano() / 1e6 
		limit := rf.lastVisited + rf.electionTimeout
        	//如果当前时间超过了规定时间，转换为candidate，并向其他所有结点发送选举请求
		if limit <= timeNow {
			...
            		rf.state = candidate;
			go rf.sendRequestVote()
		}
		rf.mu.Unlock()
		time.Sleep(1 * time.Millisecond)
	}
```

### 投票系统的设计

如何实现”当一个candidate收到超过半数结点的投票时，立刻转换为leader“？诚然，我们可以用上面设计原子钟的方法，即每隔一段时间来判断当前得票是否超过半数，然后做出决策，但是如何定义”一段时间”？是100ms？10ms还是1ms？显然，我们没有足够的理由做出选择，那么一旦某个数字被写入程序，这个数字就成了一个”Magic Number“。不过我们有更好的办法，那就是使用Condition条件变量，首先并发地请求投票，然后调用cond.wait()阻塞；一旦收到回复，立刻调用cond.broadcast()唤醒计票程序做出决策：

```go
func (rf *Raft) sendRequestVote(oldTerm int, lastIdx int, lastTerm int) {
	rf.mu.Lock()
	//初始化计票系统，采用一个mutex和一个condition
	total := len(rf.peers)
	get, visited := 1, 1 //首先投票给自己
	mu := sync.Mutex{}
	cond := sync.NewCond(&mu)
	//并发地请求投票
	for i, _ := range rf.peers {
		go func(i int) {
			...
			rf.peers[i].Call("Raft.RequestVote", &args, &reply)
			mu.Lock()
            		//收到投票，get+1
			if reply.VoteGranted {
				get++
			}
            		//总的访问量+1
			visited++
           		 //唤醒计票线程
			cond.Broadcast()
			mu.Unlock()
		}(i)
	}

	mu.Lock()
    	//如果没有收到半数的投票，将阻塞等待下一次收到回复
	for get <= total/2 && visited < total {
		cond.Wait()
	}
	...
	mu.Unlock()
}
```

### 重置原子钟的时机

在Raft论文图二中有这么一个要求：

> If election timeout elapses without receiving AppendEntries RPC from current leader or granting vote to candidate: convert to candidate

意为如果在规定时间内，如果一个follower没有收到当前leader的AppendEntries RPC或授予candidate投票:转换为candidate。这句话最核心的部分在于：“授予candidate投票”，而不是收到candidate发来的投票邀请，它们两的差别在故障较多的场景下会非常明显：在经过长时间的网络故障和leader更替后，整个集群中满足日志是最新的结点只有少数的几个，如果每收到一个投票RPC就重置原子钟，那么很有可能这几个结点将永远不会等到选举触发的那一刻，整个系统将陷入活锁，即整个系统看起来都在运行，但是永远选不出一个leader。

```go
func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	rf.mu.Lock()
	reply.Term = rf.currentTerm
	if args.Term > rf.currentTerm {
		...
		//candidate的日志比自己更新，投出自己的一票,并重置原子钟
		if lastTerm < args.LastLogTerm || (lastTerm == args.LastLogTerm && lastIdx <= args.LastLogIndex) {
           		 ...
            		reply.voteGranted = true
			rf.ResetElectionTimeout()
        	}
       		...
    	}
	rf.mu.Unlock()
}
```

### 对日志复制的优化

在Raft论文中提到了一种优化的手段：

>If desired, the protocol can be optimized to reduce the number of rejected AppendEntries RPCs. For example, when rejecting an AppendEntries request, the follower can include the term of the conﬂicting entry and the ﬁrst index it stores for that term. With this information, the leader can decrement nextIndex to bypass all of the conﬂicting entries in that term; one AppendEntries RPC will be required for each term with conﬂicting entries, rather than one RPC per entry. In practice, we doubt this optimization is necessary, since failures happen infrequently and it is unlikely that there will be many inconsistent entries. 

大意为当拒绝AppendEntries请求时，follower可以返回冲突term对应第一个索引。有了这个信息，leader可以减少nextIndex来绕过该term内所有冲突的条目，从而将算法从每个条目都需要调用一次RPC优化到每个term的所有条目只调用一次RPC。在实践中，这种优化不是必要的，因为故障很少发生，也不太可能有很多不一致的条目。但是在lab2C中，由于网络极其不可靠，所以这个优化的实现就显得尤为重要:

```go
func (rf *Raft) AppendEntry(args *AppendEntryArgs, reply *AppendEntryReply) {
	rf.mu.Lock()
	if args.Term >= rf.currentTerm {
		...
       		 //冲突发生
		if len(rf.log) <= args.PrevLogIndex || rf.log[args.PrevLogIndex].Term != args.PrevLogTerm {
			if len(rf.log) > args.PrevLogIndex {
				jt := args.PrevLogIndex
                		//获取冲突发生的term内的第一个索引
				for rf.log[jt].Term == rf.log[args.PrevLogIndex].Term {
					jt--
				}
				reply.JumpTo = jt
			} else {
				reply.JumpTo = len(rf.log)
			}
			rf.mu.Unlock()
			return
		}
		...
}
```

### 结果
```shell
Test (2A): initial election ...
  ... Passed --   4.0  3   32    9170    0
Test (2A): election after network failure ...
  ... Passed --   6.1  3   70   13895    0
Test (2B): basic agreement ...
  ... Passed --   1.6  3   18    5158    3
Test (2B): RPC byte count ...
  ... Passed --   3.3  3   50  115122   11
Test (2B): agreement despite follower disconnection ...
  ... Passed --   6.3  3   64   17489    7
Test (2B): no agreement if too many followers disconnect ...
  ... Passed --   4.9  5  116   27838    3
Test (2B): concurrent Start()s ...
  ... Passed --   2.1  3   16    4648    6
Test (2B): rejoin of partitioned leader ...
  ... Passed --   8.1  3  111   26996    4
Test (2B): leader backs up quickly over incorrect follower logs ...
  ... Passed --  28.6  5 1342  953354  102
Test (2B): RPC counts aren't too high ...
  ... Passed --   3.4  3   30    9050   12
Test (2C): basic persistence ...
  ... Passed --   7.2  3  206   42208    6
Test (2C): more persistence ...
  ... Passed --  23.2  5 1194  198270   16
Test (2C): partitioned leader and one follower crash, leader restarts ...
  ... Passed --   3.2  3   46   10638    4
Test (2C): Figure 8 ...
  ... Passed --  35.1  5 9395 1939183   25
Test (2C): unreliable agreement ...
  ... Passed --   4.2  5  244   85259  246
Test (2C): Figure 8 (unreliable) ...
  ... Passed --  36.3  5 1948 4175577  216
Test (2C): churn ...
  ... Passed --  16.6  5 4402 2220926 1766
Test (2C): unreliable churn ...
  ... Passed --  16.5  5  781  539084  221
PASS
ok      raft    203.237s
