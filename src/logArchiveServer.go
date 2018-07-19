package main

import (
	"errors"
	"flag"
	"fmt"
	"github.com/jonnywang/go-kits/redis"
	"logarchive"
	"os"
	"os/signal"
	"regexp"
	"runtime"
	"sync"
	"syscall"
	"time"
	"path"
)

var (
	ERRPARAMS = errors.New("error params")
)

const (
	VERSION = "1.2.2"
	OK      = "OK"
)

var optionConfigFile = flag.String("config", "./config.xml", "configure xml file")

func usage() {
	fmt.Printf("Version: %s\nUsage: %s [options]Options:", VERSION, os.Args[0])
	flag.PrintDefaults()
	os.Exit(0)
}

type LogFile struct {
	sync.Mutex
	Name   string
	Handle *os.File
	C      chan int
	T      time.Time
}

var LogFiles map[string]LogFile = make(map[string]LogFile)

func OpenLogFileForWrite(fileName string) (*os.File, error) {
	if err := logarchive.MkdirpByFileName(fileName); err != nil {
		redis.Logger.Print("failed to open log file: %s", err)
		return nil, err
	}

	f, err := os.OpenFile(fileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		redis.Logger.Print("failed to open log file: %s", err)
		return nil, err
	}

	return f, nil
}

func SaveLogToFile(fileName string, lineContent string) error {
	file, ok := LogFiles[fileName]
	if !ok || file.Name != fileName {
		handle, err := OpenLogFileForWrite(path.Join(logarchive.Config.Repertory, fileName))
		if err != nil {
			return err
		}

		file = LogFile{
			Name:   fileName,
			Handle: handle,
			C:      make(chan int),
			T:      time.Now(),
		}

		LogFiles[fileName] = file

		redis.Logger.Printf("reopen %s", fileName)

		go func() {
			delay := time.Second * time.Duration(logarchive.Config.Timeout)
			interval := time.NewTicker(delay)
			defer func() {
				interval.Stop()
				close(file.C)
			}()

			for {
				select {
				case <-file.C:
					file.T = time.Now().Add(delay)
				case <-interval.C:
					if file.T.Before(time.Now()) {
						redis.Logger.Printf("timeout:%s", fileName)
						file.Lock()
						defer file.Unlock()
						file.Handle.Close()
						delete(LogFiles, fileName)
						return
					}
				}
			}
		}()
	}

	file.Lock()
	defer file.Unlock()

	file.C <- 1
	n, err := file.Handle.WriteString(lineContent + "\n")
	if err != nil {
		return err
	}

	if n != len(lineContent) + 1 {
		return errors.New("not full write content to file")
	}

	return nil;
}

type LocalRedisFileHandler struct {
	sync.Mutex
	redis.RedisHandler
	c chan int
	logs chan []string
}

func (that *LocalRedisFileHandler) Init() error {
	that.logs = make(chan []string, 1024)
	that.c = make(chan int, 1)

	go func() {
		E:
		for {
			select {
			case <-that.c:
				break E
			case logContent := <- that.logs:
				redis.Logger.Printf("%v", logContent)
				if len(logContent[0]) > 0 && len(logContent[1]) > 0 {
					SaveLogToFile(logContent[0], logContent[1])
				}
			}
		}

		redis.Logger.Print("log record process exit")
	}()

	return nil
}

func (that *LocalRedisFileHandler) Shutdown() error {
	for _, v := range LogFiles {
		v.Handle.Close()
		close(v.C)
	}

	that.c <- 1

	return nil
}

func (that *LocalRedisFileHandler) Select(db int) (string, error) {
	return OK, nil
}

func (that *LocalRedisFileHandler) Version() (string, error) {
	return VERSION, nil
}

func (that *LocalRedisFileHandler) Ping(message string) (string, error)  {
	if len(message) > 0 {
		return message, nil
	}

	return "PONG", nil
}

func (that *LocalRedisFileHandler) FlushAll() (string, error) {
	return OK, nil
}

func (that *LocalRedisFileHandler) FlushDB(db int) (string, error) {
	return "", ERRPARAMS
}

func (that *LocalRedisFileHandler) Set(fileName string, lineContent string) (string, error) {
	matched, _ := regexp.MatchString("^[a-zA-Z\\-_:.0-9]{1,}$", fileName)
	if !matched {
		return "", ERRPARAMS
	}

	that.logs <- []string{fileName, lineContent}

	return OK, nil
}

func main() {
	flag.Usage = usage
	flag.Parse()

	if len(os.Args) < 2 {
		usage()
	}

	_, err := logarchive.ParseXmlConfig(*optionConfigFile)
	if err != nil {
		redis.Logger.Print(err)
		os.Exit(1)
	}

	redis.Logger.Printf("read config %+v", logarchive.Config)

	runtime.GOMAXPROCS(runtime.NumCPU())

	lrh := &LocalRedisFileHandler{}
	lrh.Initiation(func(){
		lrh.Init()
	})

	ser, err := redis.NewServer(logarchive.Config.Address, lrh)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	sigs := make(chan os.Signal)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		<-sigs
		ser.Stop(10)
		lrh.Shutdown()
	}()

	err = ser.Start()
	if err != nil {
		fmt.Println(err)
	}

	redis.Logger.Print("server shudown")

	os.Exit(0)
}
