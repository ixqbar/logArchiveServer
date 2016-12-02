# logArchiveServer

基于redis协议实现的一个日志记录服务器

### USAGE (./logArchiveServer -h)
```
Usage: ./redisArchiveServer [options]
Options:
  -dir string
    	root directory for logs data (default "./data")
  -listen string
    	server listen path, e.g ":6379" or "/var/run/logserver.sock" (default ":6379")
  -timeout uint
    	timeout to close opened files (default 60)
  -verbose int
    	show run details
```

### save log to server command
```
set log_file_path_name linecontent
```

### TEST
```
{YOUR REDIS BIN PATH}/redis-cli
set 20161202-log good
```

### FAQ
更多疑问请+qq群 233415606 or [website http://www.hnphper.com](http://www.hnphper.com)