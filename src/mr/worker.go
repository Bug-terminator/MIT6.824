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

// for sorting by key.
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

	// uncomment to send the Example RPC to the master.
	//CallExample()
	//TODO require a job
	// send finish to master
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
						//err := os.Chmod(filename, 0777)
						//if err != nil {
						//	fmt.Println(err)
						//}
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
// example function to show how to make an RPC call to the master.
//
// the RPC argument and reply types are defined in rpc.go.
//
func CallExample() {

	// declare an argument structure.
	args := ExampleArgs{}

	// fill in the argument(s).
	args.X = 99

	// declare a reply structure.
	reply := ExampleReply{}

	// send the RPC request, wait for the reply.
	call("Master.Example", &args, &reply)

	// reply.Y should be 100.
	fmt.Printf("reply.Y %v\n", reply.Y)
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
