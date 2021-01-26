# GFS

## intro

GFS is differ from traditional distributed filesystem：

- First, component failures are the norm rather than the exception.
- Second, ﬁles are huge by traditional standards.
- Third, most ﬁles are mutated by appending new data rather than overwriting existing data. **Random writes within a ﬁle are practically non-existent.

## DESIGN OVERVIEW

### Assumptions

-  The system is built from many **inexpensive commodity components** that often fail. It must constantly monitor itself and detect, tolerate, and recover promptly from component failures on a routine basis. 
- The system stores a modest number of large ﬁles. 
- The workloads primarily consist of two kinds of reads: **large streaming reads** and **small random reads.** In large streaming reads, Successive operations from the same client often read through a **contiguous** region of a ﬁle. A small random read typically reads a few KBs at some **arbitrary** oﬀset. **Performance-conscious** applications often **batch** and **sort** their small reads to advance steadily through the ﬁle rather than go back and forth. 
- The workloads also have many large, sequential writes that **append data to ﬁles**. Small writes at arbitrary positions in a ﬁle are **supported but do not have to be eﬃcient.** 
-  Our ﬁles are often used as producerconsumer queues or for many-way merging. Hundreds of producers, running one per machine, will concurrently append to a ﬁle. Atomicity with minimal synchronization overhead is essential. 
-  High sustained bandwidth is more important than low latency. Most of our target applications place a premium on processing data in bulk at a high rate, while few have stringent response time requirements for an individual read or write.

### Architecture

![image-20210121182422572](C:\Users\litang\AppData\Roaming\Typora\typora-user-images\image-20210121182422572.png)

### consistency model

#### mutation state

> mutation is an operation that changes the metadata or file contents.

![image-20210121231508983](C:\Users\litang\AppData\Roaming\Typora\typora-user-images\image-20210121231508983.png)

- [ ] append vs. "regular" append?

### lease and mutation order

- [ ] the global mutation order is defined first by the lease grant order chosen by the master, and within a lease by the serial numbers assigned by the primary?

![image-20210122000117221](C:\Users\litang\AppData\Roaming\Typora\typora-user-images\image-20210122000117221.png)

### 3.2 Data Flow

We **decouple the flow of data from the flow of control** to use the network efficiently. While control flows from the client to the primary and then to all secondaries, data is pushed linearly in a **pipelined** fashion. Our goals are to **fully utilize each machine’s network bandwidth**, avoid network bottlenecks and high-latency links, and minimize the latency to push through all the data.

### 3.3 Atomic Record Appends

the client specifies only the data. GFS appends it to the file ***at least once atomically*** (i.e., as one continuous sequence of bytes) at an offset of GFS’s choosing and returns that offset to the client. 

### 3.4 Snapshot

The snapshot operation makes a copy of a file or a directory tree (the “source”) almost **instantaneously**, while minimizing any interruptions of ongoing mutations. Our users use it to quickly create branch copies of huge data sets (and often copies of those copies, recursively), or to checkpoint the current state before experimenting with changes that can later be committed or rolled back easily.

- [ ] namespace

  > Unlike many traditional file systems, Nor does it support aliases for the same file or directory (i.e, hard or symbolic links in Unix terms). GFS logically **represents its namespace as a lookup table mapping full pathnames to metadata**. With prefix compression, this table can be efficiently represented in memory. Each node in the namespace tree (either an absolute file name or an absolute directory name) **has an associated read-write lock.**

- [ ] snapshot

- [ ] garbage collection

- [ ] lease

![image-20210122161703166](C:\Users\litang\AppData\Roaming\Typora\typora-user-images\image-20210122161703166.png)

- [x] what if i want to read a content from a server but that server's disk is crash down? How can the system detect the fault?

  master will connect to every chunkserver piriodically,if the most recent version chunk server doesn't respond, master will tell client "can't respond yet, please try later".

  

- [x] version number? 

  only when master assign a new primary, the version number changes.

- [ ] how does log and checkpoint work?

- [ ] record append?Modify data,rather than append data to the end of the file?

- [x] Split brain?network error,pirmary doesn't  respond,master reassigns a new primary,then we have two primaries.To solve this problem,use "**lease**".master and primary keep the time of lease(generally 60s),during this pioriod,if primary doesn't respond,the master will not reassign new primary.

- [ ] what if 0 or more chunkserver append record to chunks?What the relationship between consistency and these situation?

![image-20210123103815515](GFS.assets/image-20210123103815515.png)

![image-20210123103946107](GFS.assets/image-20210123103946107.png)

**gfs doesn't ensure consistency.**

# Lab1

- [x] mrapps/wc.go
- [x] mrsequential.go
- [ ] main/mrmaster.go
- [ ] main/mrworker.go
- [ ] implementation in `mr/master.go`, `mr/worker.go`, and `mr/rpc.go`.

## problem shooting

## todolist

- [ ] master data structure:mapslice/reduceslice
- [ ] worker requist
- [ ] worker finish
- [ ] assign

