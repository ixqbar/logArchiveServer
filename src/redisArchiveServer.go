package main

import (
	"fmt"
	"os"
	"flag"
	"sync"
	"errors"
	"regexp"
	"time"
	"runtime"
	"os/signal"
	"syscall"
	"logarchive"
	redis "github.com/jonnywang/go-kits/redis"
	"log"
)

var (
	ERRPARAMS = errors.New("error params")
)

const (
	VERSION   = "1.0.0"
	OK        = "OK"
)

func init()  {
	redis.Logger.SetFlags(log.Ldate | log.Ltime);
}

var optionListen  = flag.String("listen", ":6379", `server listen path, e.g ":6379" or "/var/run/logserver.sock"`)
var optionDir     = flag.String("dir", "./data", `root directory for logs data`)
var optionTimeout = flag.Uint("timeout", 60, "timeout to close opened files")

type LogFile struct {
	Name   string
	C      chan int
	Handle *os.File
	T      time.Time
}

var LogFiles map[string]LogFile = make(map[string]LogFile)

func OpenLogFileForWrite(fileName string) (*os.File, error) {
	if err := logarchive.MkdirpByFileName(fileName); err != nil {
		redis.Logger.Print("failed to open log file: %s", err)
		return nil, err
	}

	f, err := os.OpenFile(fileName, os.O_CREATE | os.O_WRONLY | os.O_APPEND, 0644)
	if err != nil {
		redis.Logger.Print("failed to open log file: %s", err)
		return nil, err
	}

	return f, nil
}

type LocalRedisFileHandler struct {
	redis.RedisHandler
	sync.Mutex
}

func (this *LocalRedisFileHandler) Init(db int) (error) {

	return nil
}

func (this *LocalRedisFileHandler) Shutdown() (error) {
	for _, v := range LogFiles {
		v.Handle.Close()
		close(v.C)
	}

	return nil
}

func (this *LocalRedisFileHandler) Select(db int) (string, error) {
	return OK, nil
}

func (this *LocalRedisFileHandler) Version() (string, error) {
	return VERSION, nil
}

func (this *LocalRedisFileHandler) FlushAll() (string, error) {
	return OK, nil
}

func (this *LocalRedisFileHandler) FlushDB(db int) (string, error) {
	return "", ERRPARAMS
}

func (this *LocalRedisFileHandler) Set(fileName string, lineContent string) (string, error) {
	matched, _ := regexp.MatchString("^[a-zA-Z\\-_:]{1,}$", fileName)
	if !matched {
		return "", ERRPARAMS
	}

	this.Lock()
	defer this.Unlock()

	file, ok := LogFiles[fileName]
	if  !ok || file.Name != fileName {
		handle, err := OpenLogFileForWrite(fmt.Sprintf("%s/%s", this.Config["path"].(string), fileName))
		if err != nil {
			return "", err
		}

		file = LogFile{
			Name   : fileName,
			Handle : handle,
			C      : make(chan int),
			T      : time.Now(),
		}

		redis.Logger.Printf("reopen %s", fileName)

		go func() {
			delay    := time.Second * time.Duration(this.Config["timeout"].(uint))
			interval := time.NewTicker(delay)
			for {
				select {
				case <-file.C:
					file.T = time.Now().Add(delay)
				case <-interval.C:
					if file.T.Before(time.Now()) {
						redis.Logger.Print("timeout:%s", fileName)
						this.Lock()
						defer this.Unlock()
						interval.Stop()
						file.Handle.Close()
						delete(LogFiles, fileName)
						close(file.C)
						return
					}
				}
			}
		}()

		LogFiles[fileName] = file
	}

	file.C <- 1
	n, err := file.Handle.WriteString(lineContent + "\n")
	if err != nil {
		return "", err
	}

	if n != len(lineContent) + 1 {
		return "", errors.New("not full write content to file")
	}

	return OK, nil
}

func usage() {
	fmt.Printf("Usage: %s [options]\nOptions:\n", os.Args[0])
	flag.PrintDefaults()
	os.Exit(0)
}

func main() {
	flag.Usage = usage
	flag.Parse()

	v, err := logarchive.GetExistsAbsolutePath(*optionDir)
	if err != nil {
		redis.Logger.Print(err)
		os.Exit(1)
	}

	runtime.GOMAXPROCS(runtime.NumCPU())

	lrh := &LocalRedisFileHandler{}

	lrh.SetConfig(map[string]interface{}{
		"path" : v,
		"timeout" : *optionTimeout,
	})

	lrh.SetShield("Init")
	lrh.SetShield("Shutdown")
	lrh.SetShield("Lock")
	lrh.SetShield("Unlock")
	lrh.SetShield("SetShield")
	lrh.SetShield("SetConfig")
	lrh.SetShield("CheckShield")

	ser, err := redis.NewServer(*optionListen, lrh)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	sigs := make(chan os.Signal)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		<-sigs
		ser.Stop(10)
	}()

	err = ser.Start()
	if err != nil {
		fmt.Println(err)
	}

	lrh.Shutdown()

	fmt.Printf("server shudown\n")

	os.Exit(0)
}
