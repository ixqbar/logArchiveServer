# logArchiveServer

基于redis协议实现的一个日志记录服务器

### USAGE (./logArchiveServer -h)
```
./logArchiveServer --config=config.xml
```

### config.xml
```
<?xml version="1.0" encoding="UTF-8" ?>
<config>
	<address>0.0.0.0:6599</address>
	<user>www-data</user>
	<group>www-data</group>
	<perm>0755</perm>
	<repertory>/data/logs</repertory>
	<timeout>30</timeout>
</config>
```

### save log to server command
```
set log_file_path_name linecontent
```

### TEST
```
{YOUR REDIS BIN PATH}/redis-cli
set 20161202.log good
```

### FAQ
更多疑问请+qq群 233415606
