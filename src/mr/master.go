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
	mpq MapQueue
	rdq ReduceQueue
}
//the timers
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
//TODO Your code here -- RPC handlers for the worker to call.
func (m *Master) Request(args *Args, reply *Reply) {
	reply.respond = true

	if m.mpq.done(){//for reduce
		//find a not_started task
		for i,tsk := range m.rdq.q{
			tsk.mu.Lock()
			if tsk.state == 0{
				tsk.state = 1
				go ReduceTimer(&tsk)
				reply.idx = i
				tsk.mu.Unlock()
				return
			}
			tsk.mu.Unlock()
		}
	}else {//for map
		//find a not_started task
		for i,tsk := range m.mpq.q{
			tsk.mu.Lock()
			if tsk.state == 0{
				tsk.state = 1
				go MapTimer(&tsk)
				reply.idx = i
				reply.fileName = tsk.fileName
				tsk.mu.Unlock()
				return
			}
			tsk.mu.Unlock()
		}
	}
	//if don't find one
	reply.sleep = true
	return
}
func (m *Master) Finish(args *Args, reply *Reply) {
	if args.Type == 0{//for map
		tsk := &m.mpq.q[args.idx]
		tsk.mu.Lock()
		if tsk.state != 2{
			m.mpq.finishNum++
			tsk.state = 2
		}
		tsk.mu.Unlock()
	}else{//for reduce
		tsk := &m.rdq.q[args.idx]
		tsk.mu.Lock()
		if tsk.state != 2{
			m.rdq.finishNum++
			tsk.state = 2
		}
		tsk.mu.Unlock()
	}
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

//TODO
// main/mrmaster.go calls Done() periodically to find out
// if the entire job has finished.
//
func (m *Master) Done() bool {
	ret := false

	// Your code here.


	return ret
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
