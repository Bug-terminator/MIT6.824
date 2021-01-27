package mr

import (
	"log"
	"strconv"
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
	//fmt.Println(args)//fixme
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

	//generate intermediate files
	for i := 0;i<m.mpq.totalNum;i++{
		for j:=0;j<m.rdq.totalNum;j++{
			filename := "mr-"+strconv.Itoa(i)+"-"+strconv.Itoa(j)
			os.Create(filename)
		}
	}
	//generate reduce output files
	for j:=0;j<m.rdq.totalNum;j++{
		filename := "mr-out-"+strconv.Itoa(j)
		os.Create(filename)
	}
	m.server()
	return &m
}
