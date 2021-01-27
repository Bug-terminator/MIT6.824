### Lab1

#### Time spent

This lab spent about one week, including about two days to learn GO, one day to read papers and courses, two days to write code and one day to debug.

#### Task description

Your job is to implement a distributed MapReduce, consisting of two programs, the master and the worker. There will be just one master process, and one or more worker processes executing in parallel. In a real system the workers would run on a bunch of different machines, but for this lab you'll run them all on a single machine. The workers will talk to the master via RPC. Each worker process will ask the master for a task, read the task's input from one or more files, execute the task, and write the task's output to one or more files. The master should notice if a worker hasn't completed its task in a reasonable amount of time (for this lab, use ten seconds), and give the same task to a different worker.

We have given you a little code to start you off. The "main" routines for the master and worker are in `main/mrmaster.go` and `main/mrworker.go`; don't change these files. You should put your implementation in `mr/master.go`, `mr/worker.go`, and `mr/rpc.go`.

#### source code reading

- [x] mrapps/wc.go
- [x] mrsequential.go
- [x] main/mrmaster.go
- [x] main/mrworker.go
- [x] implementation in `mr/master.go`, `mr/worker.go`, and `mr/rpc.go`.

#### A few rules:

- The map phase should divide the intermediate keys into buckets for `nReduce` reduce tasks, where `nReduce` is the argument that `main/mrmaster.go` passes to `MakeMaster()`.
- The worker implementation should put the output of the X'th reduce task in the file `mr-out-X`.
- A `mr-out-X` file should contain one line per Reduce function output. The line should be generated with the Go `"%v %v"` format, called with the key and value. Have a look in `main/mrsequential.go` for the line commented "this is the correct format". The test script will fail if your implementation deviates too much from this format.
- You can modify `mr/worker.go`, `mr/master.go`, and `mr/rpc.go`. You can temporarily modify other files for testing, but make sure your code works with the original versions; we'll test with the original versions.
- The worker should put intermediate Map output in files in the current directory, where your worker can later read them as input to Reduce tasks.
- `main/mrmaster.go` expects `mr/master.go` to implement a `Done()` method that returns true when the MapReduce job is completely finished; at that point, `mrmaster.go` will exit.
- When the job is completely finished, the worker processes should exit. A simple way to implement this is to use the return value from `call()`: if the worker fails to contact the master, it can assume that the master has exited because the job is done, and so the worker can terminate too. Depending on your design, you might also find it helpful to have a "please exit" pseudo-task that the master can give to workers.

#### Hints

- One way to get started is to modify `mr/worker.go`'s `Worker()` to send an RPC to the master asking for a task. Then modify the master to respond with the file name of an as-yet-unstarted map task. Then modify the worker to read that file and call the application Map function, as in `mrsequential.go`.

- The application Map and Reduce functions are loaded at run-time using the Go plugin package, from files whose names end in `.so`.

- If you change anything in the `mr/` directory, you will probably have to re-build any MapReduce plugins you use, with something like `go build -buildmode=plugin ../mrapps/wc.go`

- This lab relies on the workers sharing a file system. That's straightforward when all workers run on the same machine, but would require a global filesystem like GFS if the workers ran on different machines.

- A reasonable naming convention for intermediate files is `mr-X-Y`, where X is the Map task number, and Y is the reduce task number.

- The worker's map task code will need a way to store intermediate key/value pairs in files in a way that can be correctly read back during reduce tasks. One possibility is to use Go's encoding/json package. To write key/value pairs to a JSON file:

  ```
    enc := json.NewEncoder(file)
    for _, kv := ... {
      err := enc.Encode(&kv)
  ```

  and to read such a file back:

  ```
    dec := json.NewDecoder(file)
    for {
      var kv KeyValue
      if err := dec.Decode(&kv); err != nil {
        break
      }
      kva = append(kva, kv)
    }
  ```

- The map part of your worker can use the `ihash(key)` function (in `worker.go`) to pick the reduce task for a given key.

- You can steal some code from `mrsequential.go` for reading Map input files, for sorting intermedate key/value pairs between the Map and Reduce, and for storing Reduce output in files.

- The master, as an RPC server, will be concurrent; don't forget to lock shared data.

- Use Go's race detector, with `go build -race` and `go run -race`. `test-mr.sh` has a comment that shows you how to enable the race detector for the tests.

- Workers will sometimes need to wait, e.g. reduces can't start until the last map has finished. One possibility is for workers to periodically ask the master for work, sleeping with `time.Sleep()` between each request. Another possibility is for the relevant RPC handler in the master to have a loop that waits, either with `time.Sleep()` or `sync.Cond`. Go runs the handler for each RPC in its own thread, so the fact that one handler is waiting won't prevent the master from processing other RPCs.

- The master can't reliably distinguish between crashed workers, workers that are alive but have stalled for some reason, and workers that are executing but too slowly to be useful. The best you can do is have the master wait for some amount of time, and then give up and re-issue the task to a different worker. For this lab, have the master wait for ten seconds; after that the master should assume the worker has died (of course, it might not have).

- To test crash recovery, you can use the `mrapps/crash.go` application plugin. It randomly exits in the Map and Reduce functions.

- To ensure that nobody observes partially written files in the presence of crashes, the MapReduce paper mentions the trick of using a temporary file and atomically renaming it once it is completely written. You can use `ioutil.TempFile` to create a temporary file and `os.Rename` to atomically rename it.

- `test-mr.sh` runs all the processes in the sub-directory `mr-tmp`, so if something goes wrong and you want to look at intermediate or output files, look there.

### Problem shooting

#### Todolist

- [x] master data structure
- [x] RPC
- [x] worker requist
- [x] worker finish
- [x] master assign
- [ ] timer:I don't really realize timer,but the test still passed.

#### Code

```go
//rpc.go
package mr

//
// RPC definitions.
//
// remember to capitalize all names.
//

import "os"
import "strconv"

// Add your RPC definitions here.
type Args struct {
	Type, Idx int
}

type Reply struct {
	Respond, Sleep bool
	Idx,NReduce,NMap int
	FileName string
}
// Cook up a unique-ish UNIX-domain socket name
// in /var/tmp, for the master.
// Can't use the current directory since
// Athena AFS doesn't support UNIX-domain sockets.
func masterSock() string {
	s := "/var/tmp/824-mr-"
	s += strconv.Itoa(os.Getuid())
	return s
}
```

```go
//worker.go
package mr

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"sort"
	"strconv"
	"time"
)
import "log"
import "net/rpc"
import "hash/fnv"

//
// Map functions return a slice of KeyValue.
//
type KeyValue struct {
	Key   string
	Value string
}

// for sorting by key.
type ByKey []KeyValue
func (a ByKey) Len() int           { return len(a) }
func (a ByKey) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByKey) Less(i, j int) bool { return a[i].Key < a[j].Key }

//

// use ihash(key) % NReduce to choose the reduce
// task number for each KeyValue emitted by Map.
//
func ihash(key string) int {
	h := fnv.New32a()
	h.Write([]byte(key))
	return int(h.Sum32() & 0x7fffffff)
}

//
// main/mrworker.go calls this function.
//
func Worker(mapf func(string, string) []KeyValue,
	reducef func(string, []string) string) {
	// Your worker implementation here.
	for {
		args := Args{}
		reply := Reply{}
		call("Master.Request", &args, &reply)
		if !reply.Respond { //master doesn't respond
			break
		} else {
			if reply.Sleep { //need to sleep
				time.Sleep(2 * time.Second)
			} else {                      //normal task
				if reply.FileName != "" { //map task
					// create temp files and its correlated encoders,you can also use map
					openfileQ := make([]*os.File, reply.NReduce, reply.NReduce)
					encoderQ := make([]*json.Encoder, reply.NReduce, reply.NReduce)
					for i := 0; i < reply.NReduce; i++ {
						tmp, err := ioutil.TempFile(".", "Inter")
						if err != nil {
							log.Fatalf("cannot create temp file")
						}
						openfileQ[i] = tmp
						encoderQ[i] = json.NewEncoder(tmp)
					}

					//get content,invoke map function and get kva
					file, err := os.Open(reply.FileName)
					if err != nil {
						log.Fatalf("cannot open %v", reply.FileName)
					}
					content, err := ioutil.ReadAll(file)
					if err != nil {
						log.Fatalf("cannot read %v", reply.FileName)
					}
					file.Close()
					kva := mapf(reply.FileName, string(content))

					//hash key,then encode them to corraleted file
					for _, kv := range kva {
						hashKey := ihash(kv.Key) % reply.NReduce
						encoderQ[hashKey].Encode(&kv)
					}

					//rename && close files,notice that close should be done before renaming
					for i, f:= range openfileQ {
						filename := "mr-" + strconv.Itoa(reply.Idx) + "-" + strconv.Itoa(i)
						fname := f.Name()
						f.Close()
						os.Rename(fname, filename)
					}
					//send finish to master
					args.Idx = reply.Idx
					args.Type = 0
					call("Master.Finish", &args, &reply)
				} else { //reduce task
					//get content from correlated files
					intermediate := []KeyValue{}
					for i := 0; i < reply.NMap; i++ {
						filename := "mr-" + strconv.Itoa(i) + "-" + strconv.Itoa(reply.Idx)
						file, err := os.Open(filename)
						if err != nil {
							log.Fatalf("cannot open %v", filename)
						}
						dec := json.NewDecoder(file)
						//append kv-pairs to intermediate
						for {
							var kv KeyValue
							if err := dec.Decode(&kv); err != nil {
								break
							}
							intermediate = append(intermediate, kv)
						}
						file.Close()
					}
					//shuffle
					sort.Sort(ByKey(intermediate))

					//create tmp output file
					tmp, err := ioutil.TempFile(".", "Out")
					if err != nil {
						log.Fatalf("cannot create temp file")
					}

					//
					// call Reduce on each distinct key in intermediate[],
					// and print the result to mr-out-X.
					//
					i := 0
					for i < len(intermediate) {
						j := i + 1
						for j < len(intermediate) && intermediate[j].Key == intermediate[i].Key {
							j++
						}
						values := []string{}
						for k := i; k < j; k++ {
							values = append(values, intermediate[k].Value)
						}
						output := reducef(intermediate[i].Key, values)

						// this is the correct format for each line of Reduce output.
						fmt.Fprintf(tmp, "%v %v\n", intermediate[i].Key, output)

						i = j
					}
					//rename
					fname := tmp.Name()
					tmp.Close()
					oname := "mr-out-" + strconv.Itoa(reply.Idx)
					os.Rename(fname, oname)
					//send finish to master
					args.Idx = reply.Idx
					args.Type = 1
					call("Master.Finish", &args, &reply)
				}
			}
		}
	}
}

//
// send an RPC request to the master, wait for the response.
// usually returns true.
// returns false if something goes wrong.
//
func call(rpcname string, args interface{}, reply interface{}) bool {
	// c, err := rpc.DialHTTP("tcp", "127.0.0.1"+":1234")
	sockname := masterSock()
	c, err := rpc.DialHTTP("unix", sockname)
	if err != nil {
		log.Fatal("dialing:", err)
	}
	defer c.Close()

	err = c.Call(rpcname, args, reply)
	if err == nil {
		return true
	}

	fmt.Println(err)
	return false
}
```

```go
//master.go
package mr

import (
	"log"
	"sync"
	"time"
)
import "net"
import "os"
import "net/rpc"
import "net/http"

//map task
type MapTask struct {
	mu sync.Mutex
	fileName string
	state int // 0/1/2 not_started/undergoing/finish
}
//map queue
type MapQueue struct {
	finishNum, totalNum int
	q []MapTask
}

//check if done
func ( m *MapQueue) done() bool{
	return m.finishNum == m.totalNum
}

//reduce task
type ReduceTask struct {
	mu sync.Mutex
	state int
}
//reduce queue
type ReduceQueue struct {
	finishNum, totalNum int
	q []ReduceTask
}
func ( m *ReduceQueue) done() bool{

	return m.finishNum == m.totalNum
}

type Master struct {
	// Your definitions here.
	mu sync.Mutex
	mpq MapQueue
	rdq ReduceQueue
}
//the timers to count 10s
func MapTimer(t *MapTask){
	time.Sleep(10 *time.Second)
	t.mu.Lock()
	if t.state != 2{
		t.state = 0
	}
	t.mu.Unlock()
}
func ReduceTimer(t *ReduceTask){
	time.Sleep(10*time.Second)
	t.mu.Lock()
	if t.state != 2{
		t.state = 0
	}
	t.mu.Unlock()
}
// Your code here -- RPC handlers for the worker to call.
func (m *Master) Request(args *Args, reply *Reply) error{
	m.mu.Lock()
	defer m.mu.Unlock()
	reply.Respond = true
	if m.mpq.done(){//for reduce
		//find a not_started task
		for i,tsk := range m.rdq.q{
			if tsk.state == 0{
				tsk.state = 1
				//go ReduceTimer(&tsk)todo
				reply.Idx = i
				reply.NMap = m.mpq.totalNum
				return nil
			}
		}
	}else {//for map
		//find a not_started task
		for i,tsk := range m.mpq.q{
			if tsk.state == 0{
				tsk.state = 1
				//go MapTimer(&tsk)todo
				reply.Idx = i
				reply.NReduce = m.rdq.totalNum
				reply.FileName = tsk.fileName
				return nil
			}
		}
	}
	//if don't find one
	reply.Sleep = true
	return nil
}
func (m *Master) Finish(args *Args, reply *Reply) error{
	m.mu.Lock()
	defer m.mu.Unlock()
	if args.Type == 0{//for map
		tsk := &m.mpq.q[args.Idx]
		tsk.mu.Lock()
		if tsk.state != 2{
			m.mpq.finishNum++
			tsk.state = 2
		}
		tsk.mu.Unlock()
	}else{//for reduce
		tsk := &m.rdq.q[args.Idx]
		tsk.mu.Lock()
		if tsk.state != 2{
			m.rdq.finishNum++
			tsk.state = 2
		}
		tsk.mu.Unlock()
	}
	return nil
}

//
// an example RPC handler.
//
// the RPC argument and reply types are defined in rpc.go.
//
func (m *Master) Example(args *ExampleArgs, reply *ExampleReply) error {
	reply.Y = args.X + 1
	return nil
}


//
// start a thread that listens for RPCs from worker.go
//
func (m *Master) server() {
	rpc.Register(m)
	rpc.HandleHTTP()
	//l, e := net.Listen("tcp", ":1234")
	sockname := masterSock()
	os.Remove(sockname)
	l, e := net.Listen("unix", sockname)
	if e != nil {
		log.Fatal("listen error:", e)
	}
	go http.Serve(l, nil)
}

//
// main/mrmaster.go calls Done() periodically to find out
// if the entire job has finished.
//
func (m *Master) Done() bool {
	// Your code here.
	return 	m.rdq.done() && m.mpq.done()
}

//
// create a Master.
// main/mrmaster.go calls this function.
// nReduce is the number of reduce tasks to use.
//
func MakeMaster(files []string, nReduce int) *Master {
	m := Master{}

	// Your code here.

	// init map queue
	m.mpq.totalNum = len(files)
	for _,file := range files{
		newTask := MapTask{fileName:file}
		m.mpq.q = append(m.mpq.q, newTask)
	}
	// init reduce queue
	m.rdq.totalNum = nReduce
	m.rdq.q = make([]ReduceTask, nReduce)

	m.server()
	return &m
}
```

#### Result

```shell
litang@LAPTOP-A13E53DB:/mnt/c/Users/litang/githubWorkSpace/MIT-6.824/src/main$ sh test-mr.sh
--- wc test: PASS
--- indexer test: PASS
--- map parallelism test: PASS
--- reduce parallelism test: PASS
--- crash test: PASS
*** PASSED ALL TESTS
```

#### Gains and Reflections

Through this lab, I have made a lot of progress: I learned a language quickly in a short time and designed a distributed system then debugged with it, both tough to achive.

This is a reiteration of Google's MapReduce paper,and this will be my most valuable learning experience. I am looking forward to the challenges in the subsequent experiments.