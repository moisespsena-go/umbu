package cache

import (
	"fmt"
	"sync"

	"github.com/moisespsena/go-assetfs/api"
	"github.com/moisespsena/template/text/template"
)

type ExecutorCache struct {
	Enable bool
	data   sync.Map
}

func NewCache() *ExecutorCache {
	return &ExecutorCache{}
}

var Cache = NewCache()

func (ec *ExecutorCache) Load(name string) *template.Executor {
	v, ok := ec.data.Load(name)
	if !ok {
		return nil
	}
	return v.(*template.Executor)
}

func (ec *ExecutorCache) LoadOrStore(name string, loader func(name string) (*template.Executor, error)) (*template.Executor, error) {
	if ec.Enable {
		v, ok := ec.data.Load(name)
		if !ok {
			v, err := loader(name)
			if err != nil {
				return nil, err
			}
			if v == nil {
				return nil, fmt.Errorf("nil value")
			}
			ec.data.Store(name, v)
			return v, nil
		}
		return v.(*template.Executor), nil
	}
	return loader(name)
}

func (ec *ExecutorCache) LoadOrStoreInfo(info api.FileInfo, loader func(info api.FileInfo) (*template.Executor, error)) (*template.Executor, error) {
	if ec.Enable {
		v, ok := ec.data.Load(info)
		if !ok {
			v, err := loader(info)
			if err != nil {
				return nil, err
			}
			if v == nil {
				return nil, fmt.Errorf("nil value")
			}
			ec.data.Store(info.RealPath(), v)
			return v, nil
		}
		return v.(*template.Executor), nil
	}
	return loader(info)
}

func (ec *ExecutorCache) LoadOrStoreNames(name string, loader func(name string) (*template.Executor, error), names ...string) (*template.Executor, error) {
	names = append([]string{name}, names...)
	for _, name := range names {
		v, ok := ec.data.Load(name)
		if ok && v != nil {
			return v.(*template.Executor), nil
		}

		t, err := loader(name)

		if err != nil {
			return nil, err
		}

		if t != nil {
			if ec.Enable {
				ec.data.Store(name, t)
			}
			return t, nil
		}
	}
	return nil, nil
}

func (ec *ExecutorCache) LoadOrStoreInfos(info api.FileInfo, loader func(info api.FileInfo) (*template.Executor, error), infos ...api.FileInfo) (*template.Executor, error) {
	infos = append([]api.FileInfo{info}, infos...)
	for _, info := range infos {
		v, ok := ec.data.Load(info.RealPath())
		if ok && v != nil {
			return v.(*template.Executor), nil
		}

		t, err := loader(info)

		if err != nil {
			return nil, err
		}

		if t != nil {
			if ec.Enable {
				ec.data.Store(info.RealPath(), t)
			}
			return t, nil
		}
	}
	return nil, nil
}
