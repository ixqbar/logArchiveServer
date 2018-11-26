package logarchive

import (
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/user"
	"strconv"
)

type ArchiveFileConfig struct {
	Uid int
	Gid int
	Perm os.FileMode
}

type ArchiveConfig struct {
	Address   string `xml:"address"`
	UserStr string `xml:"user"`
	GroupStr string `xml:"group"`
	PermStr string `xml:"perm"`
	Repertory string `xml:"repertory"`
	Timeout   int    `xml:"timeout"`
	FileConfig *ArchiveFileConfig
}

var Config *ArchiveConfig

func ParseXmlConfig(path string) (*ArchiveConfig, error) {
	if len(path) == 0 {
		return nil, errors.New("not found configure xml file")
	}

	n, err := GetFileSize(path)
	if err != nil || n == 0 {
		return nil, errors.New("not found configure xml file")
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	Config = &ArchiveConfig{}

	data := make([]byte, n)

	m, err := f.Read(data)
	if err != nil {
		return nil, err
	}

	if int64(m) != n {
		return nil, errors.New(fmt.Sprintf("expect read configure xml file size %d but result is %d", n, m))
	}

	err = xml.Unmarshal(data, &Config)
	if err != nil {
		return nil, err
	}

	r, err := CheckFileIsDirectory(Config.Repertory)
	if !r {
		return nil, err
	}

	u, err := user.Lookup(Config.UserStr)
	if err != nil {
		return nil, fmt.Errorf("error user config %s", Config.UserStr)
	}

	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return nil, fmt.Errorf("error user config %s", Config.UserStr)
	}

	g, err := user.LookupGroup(Config.GroupStr)
	if err != nil {
		return nil, fmt.Errorf("error group config %s", Config.GroupStr)
	}

	gid, err := strconv.Atoi(g.Gid)
	if err != nil {
		return nil, fmt.Errorf("error group config %s", Config.GroupStr)
	}

	perm, err := strconv.ParseInt(Config.PermStr, 0, 32)
	if err != nil {
		return nil, fmt.Errorf("error perm config %s", Config.PermStr)
	}

	Config.FileConfig = &ArchiveFileConfig{
		Uid:uid,
		Gid:gid,
		Perm:os.FileMode(perm),
	}

	return Config, nil
}
