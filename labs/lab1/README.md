### Lab1

#### Time spent

This lab spent about 4 days, including about one day to learn GO, one day to read papers and courses, two days to write code and debug.

#### Task description

Your job is to implement a distributed MapReduce, consisting of two programs, the master and the worker. There will be just one master process, and one or more worker processes executing in parallel. In a real system the workers would run on a bunch of different machines, but for this lab you'll run them all on a single machine. The workers will talk to the master via RPC. Each worker process will ask the master for a task, read the task's input from one or more files, execute the task, and write the task's output to one or more files. The master should notice if a worker hasn't completed its task in a reasonable amount of time (for this lab, use ten seconds), and give the same task to a different worker.

We have given you a little code to start you off. The "main" routines for the master and worker are in `main/mrmaster.go` and `main/mrworker.go`; don't change these files. You should put your implementation in `mr/master.go`, `mr/worker.go`, and `mr/rpc.go`.

#### source code reading

- [x] mrapps/wc.go
- [x] mrsequential.go
- [x] main/mrmaster.go
- [x] main/mrworker.go
- [x] implementation in `mr/master.go`, `mr/worker.go`, and `mr/rpc.go`.

#### Todolist

- [x] master data structure
- [x] RPC
- [x] worker requist
- [x] worker finish
- [x] master assign

#### Code

```go
//rpc.go

// Add your RPC definitions here.
type Args struct {
	Type, Idx int
}

type Reply struct {
	Respond, Sleep bool
	Idx,NReduce,NMap int
	FileName string
}
```

```go
//worker.go

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
```

```go
//master.go

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

Through this lab, I have made a lot of progress: I learned a language quickly in a short time and designed a distributed system then debugged with it, tough but interesting.I am looking forward to the challenges in the subsequent experiments.
