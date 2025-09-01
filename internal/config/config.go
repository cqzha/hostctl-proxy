package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"

	"dario.cat/mergo"
)

type AppCfg struct {
	Socket      bool     `json:"socket"`
	Websocket   bool     `json:"websocket"`
	Executor    string   `json:"executor"`
	RootPath    string   `json:"root_path"`
	DefaultArgs []string `json:"default_args"`
	MaxRetries  int      `json:"max_retries"`
	Shell       bool     `json:"shell"`
	OnStart     string   `json:"on_start"`
	OnStop      string   `json:"on_stop"`
}

type CmdCfg struct {
	Cmd         string   `json:"cmd"`
	DefaultArgs []string `json:"default_args"`
}

type SysCfg struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

// 暂时留着做http的转发
type ProxyCfg struct {
	Socket  bool                   `json:"socket"`
	Host    string                 `json:"host"`
	Port    int                    `json:"port"`
	Url     string                 `json:"url"`
	Setting map[string]interface{} `json:"setting"`
}

type ServerConfig struct {
	rl      sync.RWMutex
	sys     *SysCfg
	proxies map[string]*ProxyCfg
	cmds    map[string]*CmdCfg
	apps    map[string]*AppCfg
}

func New() *ServerConfig {
	return &ServerConfig{
		sys:     &SysCfg{},
		proxies: make(map[string]*ProxyCfg),
		cmds:    make(map[string]*CmdCfg),
		apps:    make(map[string]*AppCfg),
	}
}

func (cfg *ServerConfig) Init(filePath string) error {
	fileData, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	cfg.rl.RLock()
	defer cfg.rl.RUnlock()

	marshalData := make(map[string]*json.RawMessage, 3)
	if err = json.Unmarshal(fileData, &marshalData); err != nil {
		return err
	}

	if err = json.Unmarshal(*marshalData["sys"], cfg.sys); err != nil {
		return err
	}

	if err = json.Unmarshal(*marshalData["proxy"], &(cfg.proxies)); err != nil {
		return err
	}

	if err = json.Unmarshal(*marshalData["command"], &(cfg.cmds)); err != nil {
		return err
	}

	if err = json.Unmarshal(*marshalData["app"], &(cfg.apps)); err != nil {
		return err
	}

	return nil
}

func (cfg *ServerConfig) Exists(field string, name string) bool {
	cfg.rl.RLock()
	defer cfg.rl.RUnlock()
	if field == "command" {
		_, exist := cfg.cmds[name]
		return exist
	} else if field == "app" {
		_, exist := cfg.apps[name]
		return exist
	} else {
		_, exist := cfg.proxies[name]
		return exist
	}
}

func (cfg *ServerConfig) Add(field string, name string, data []byte) error {
	if cfg.Exists(field, name) {
		return fmt.Errorf("%s is already configured", name)
	}

	cfg.rl.RLock()
	defer cfg.rl.RUnlock()

	if field == "command" {
		var temp CmdCfg
		if err := json.Unmarshal(data, &temp); err != nil {
			return err
		}
		cfg.cmds[name] = &temp
	} else if field == "app" {
		var temp AppCfg
		if err := json.Unmarshal(data, &temp); err != nil {
			return err
		}
		cfg.apps[name] = &temp
	} else if field == "proxy" {
		var temp ProxyCfg
		if err := json.Unmarshal(data, &temp); err != nil {
			return err
		}
		cfg.proxies[name] = &temp
	} else {
		return errors.New("field error")
	}

	return nil
}

func (cfg *ServerConfig) Delete(field string, name string) error {
	if !cfg.Exists(field, name) {
		return fmt.Errorf("%s not found", name)
	}

	cfg.rl.RLock()
	defer cfg.rl.RUnlock()

	if field == "command" {
		delete(cfg.cmds, name)
	} else if field == "app" {
		delete(cfg.apps, name)
	} else if field == "proxy" {
		delete(cfg.proxies, name)
	}
	return nil
}

func (cfg *ServerConfig) Modify(field string, name string, data []byte) error {
	if !cfg.Exists(field, name) {
		return fmt.Errorf("%s not found", name)
	}

	cfg.rl.RLock()
	defer cfg.rl.RUnlock()

	if field == "command" {
		var md CmdCfg
		if err := json.Unmarshal(data, &md); err != nil {
			return err
		}
		if err := mergo.Merge(cfg.cmds[name], md, mergo.WithOverride); err != nil {
			return err
		}
	} else if field == "app" {
		var md AppCfg
		if err := json.Unmarshal(data, &md); err != nil {
			return err
		}
		if err := mergo.Merge(cfg.apps[name], md, mergo.WithOverride); err != nil {
			return err
		}
		cfg.apps[name].DefaultArgs = md.DefaultArgs
	} else if field == "proxy" {
		var md ProxyCfg
		if err := json.Unmarshal(data, &md); err != nil {
			return err
		}
		if err := mergo.Merge(cfg.proxies[name], md, mergo.WithOverride); err != nil {
			return err
		}
	}

	return nil
}

func (cfg *ServerConfig) Dump(filePath string) error {
	dump := make(map[string]interface{}, 3)
	dump["sys"] = cfg.sys
	dump["command"] = cfg.cmds
	dump["app"] = cfg.apps
	dump["proxy"] = cfg.proxies
	data, err := json.Marshal(dump)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filePath, data, 0666); err != nil {
		return err
	}
	return nil
}

func (cfg *ServerConfig) GetSysConfig() *SysCfg {
	cfg.rl.RLock()
	defer cfg.rl.RUnlock()
	return cfg.sys
}

func (cfg *ServerConfig) GetConfig(field string, name string) interface{} {
	if !cfg.Exists(field, name) {
		return nil
	}

	cfg.rl.RLock()
	defer cfg.rl.RUnlock()

	if field == "command" {
		return cfg.cmds[name]
	} else if field == "app" {
		return cfg.apps[name]
	} else if field == "proxy" {
		return cfg.proxies[name]
	}

	return nil
}

func (cfg *ServerConfig) List(field string) map[string]interface{} {
	var length int
	cfg.rl.RLock()
	defer cfg.rl.RUnlock()

	if field == "app" {
		length = len(cfg.apps)
	} else if field == "command" {
		length = len(cfg.cmds)
	} else if field == "proxy" {
		length = len(cfg.proxies)
	} else {
		length = len(cfg.apps) + len(cfg.cmds) + len(cfg.proxies)
	}
	list := make(map[string]interface{}, length)

	if field == "app" || field == "all" {
		for k, v := range cfg.apps {
			list[k] = v
		}
	}

	if field == "command" || field == "all" {
		for k, v := range cfg.cmds {
			list[k] = v
		}
	}

	if field == "proxy" || field == "all" {
		for k, v := range cfg.proxies {
			list[k] = v
		}
	}
	return list
}

type JsonConfig struct {
	objmap map[string]*json.RawMessage
}

func NewConfig() *JsonConfig {
	return &JsonConfig{
		objmap: make(map[string]*json.RawMessage, 2),
	}
}

func (cfg *JsonConfig) MapRawMessage(filePath string) error {
	fileData, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	if err = json.Unmarshal(fileData, &cfg.objmap); err != nil {
		return err
	}

	return nil
}

func (cfg JsonConfig) GetConfig(keyname string, dst any) error {
	if err := json.Unmarshal(*cfg.objmap[keyname], dst); err != nil {
		return err
	}

	return nil
}
