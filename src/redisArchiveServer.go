package main

import (
	"logarchive"
	"fmt"
	"os"
	"flag"
	"sync"
	"errors"
	"regexp"
	"time"
)

var (
	ERRPARAMS = errors.New("error params")
)

const (
	VERSION   = "1.0.0"
	OK        = "OK"
)

var optionListen  = flag.String("listen", ":6379", `server listen path, e.g ":6379" or "/var/run/logserver.sock"`)
var optionDir     = flag.String("dir", "./data", `root directory for logs data`)
var optionVerbose = flag.Int("verbose", 0, `show run details`)

type LogFile struct {
	Name   string
	C      chan string
	Handle *os.File
}

var LogFiles map[string]LogFile = make(map[string]LogFile)

func OpenLogFileForWrite(fileName string) (*os.File, error) {
	if err := logarchive.MkdirpByFileName(fileName); err != nil {
		logarchive.Debugf("failed to open log file: %s", err)
		return nil, err
	}

	f, err := os.OpenFile(fileName, os.O_CREATE | os.O_WRONLY | os.O_APPEND, 0644)
	if err != nil {
		logarchive.Debugf("failed to open log file: %s", err)
		return nil, err
	}

	return f, nil
}

type LocalRedisFileHandler struct {
	logarchive.RedisHandler
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
	this.Lock()
	defer this.Unlock()

	matched, _ := regexp.MatchString("[a-zA-Z\\-_:]{1,}", fileName)
	if !matched {
		return "", ERRPARAMS
	}

	file := LogFiles[fileName]

	if file.Name != fileName {
		handle, err := OpenLogFileForWrite(fmt.Sprintf("%s/%s", this.Config["path"].(string), fileName))
		if err != nil {
			return "", err
		}

		file = LogFile{
			Name   : fileName,
			Handle : handle,
			C      : make(chan string),
		}

		logarchive.Debugf("reopen %s", fileName)

		go func() {
			for {
				select {
				case data := <-file.C:
					logarchive.Debugf("receive:%s", data)
				case <-time.After(time.Second * 30):
					logarchive.Debugf("timeout:%s", fileName)
					this.Lock()
					defer this.Unlock()
					file.Handle.Close()
					delete(LogFiles, fileName)
					close(file.C)
					return
				}
			}
		}()

		LogFiles[fileName] = file
	}

	file.C <- fmt.Sprintf("%s -> %s", fileName, lineContent)
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
	fmt.Printf("%s\n", `
Usage: redisFielServer [options]
Options:
	`)
	flag.PrintDefaults()
	os.Exit(0)
}

func main() {
	flag.Usage = usage
	flag.Parse()

	v, err := logarchive.GetExistsAbsolutePath(*optionDir)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	if (*optionVerbose == 1) {
		os.Setenv("DEBUG", "ok")
	}

	lrh := &LocalRedisFileHandler{}

	lrh.SetConfig(map[string]interface{}{
		"path" : v,
	})

	lrh.SetShield("Init")
	lrh.SetShield("Shutdown")
	lrh.SetShield("Lock")
	lrh.SetShield("Unlock")
	lrh.SetShield("SetShield")
	lrh.SetShield("SetConfig")

	ser, err := logarchive.NewServer(*optionListen, lrh, lrh.Shield)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	ser.ListenAndServe()

	lrh.Shutdown()
}
