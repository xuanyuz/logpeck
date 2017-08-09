package logpeck

import (
	"errors"
	sjson "github.com/bitly/go-simplejson"
)

type PeckTaskConfig struct {
	Name     string
	LogPath  string
	ESConfig ElasticSearchConfig

	LogFormat  string
	FilterExpr string
	Fields     []PeckField
	Delimiters string
}

type PeckField struct {
	Name  string
	Value string
}

type ElasticSearchConfig struct {
	Hosts   []string
	Index   string
	Type    string
	Mapping string
}

type PeckTaskStat struct {
	Name        string
	LogPath     string
	LinesPerSec int64
	BytesPerSec int64
	LinesTotal  int64
	BytesTotal  int64
	Stop        bool
}

type Stat struct {
	Name        string
	LinesPerSec int64
	BytesPerSec int64
	LinesTotal  int64
	BytesTotal  int64
}

type LogStat struct {
	LogPath         string
	PeckTaskConfigs []PeckTaskConfig
	PeckTaskStats   []PeckTaskStat
}

type PeckerStat struct {
	Name     string
	Stat     Stat
	LogStats []LogStat
}

func GetString(j *sjson.Json, key string, required bool) (string, error) {
	valJson := j.Get(key)

	if valJson.Interface() == nil {
		if required {
			return "", errors.New("Parse error: need field " + key)
		} else {
			return "", nil
		}
	}
	return valJson.String()
}

func GetStringArray(j *sjson.Json, key string) ([]string, error) {
	valJson := j.Get(key)

	if valJson.Interface() == nil {
		return []string{""}, errors.New("Parse error: need field " + key)
	}
	return valJson.StringArray()
}

func ParseESConfig(j *sjson.Json) (config ElasticSearchConfig, e error) {
	cJson := j.Get("ESConfig")
	if cJson.Interface() == nil {
		return config, nil
	}

	// Parse "ESConfig.Hosts", required
	config.Hosts, e = GetStringArray(cJson, "Hosts")
	if e != nil {
		return
	}
	// Parse "ESConfig.Index", required
	config.Index, e = GetString(cJson, "Index", true)
	if e != nil {
		return
	}
	// Parse "ESConfig.Type", required
	config.Type, e = GetString(cJson, "Type", true)
	if e != nil {
		return
	}

	// Parse "ESConfig.Mapping", optional
	mapping, _ := cJson.Get("Mapping").Encode()
	config.Mapping = string(mapping[:])
	return config, nil
}

func (p *PeckTaskConfig) Unmarshal(jsonStr []byte) (e error) {
	j, je := sjson.NewJson(jsonStr)
	if je != nil {
		return je
	}

	// Parse "Name", required
	p.Name, e = GetString(j, "Name", true)
	if e != nil {
		return e
	}
	// Parse "LogPath", optional
	p.LogPath, e = GetString(j, "LogPath", false)
	if e != nil {
		return e
	}
	// Parse "ESConfig", optional
	p.ESConfig, e = ParseESConfig(j)
	if e != nil {
		return e
	}

	// Parse "LogFormat", optional
	p.LogFormat, e = GetString(j, "LogFormat", false)
	if e != nil {
		return e
	}

	// Parse "FilterExpr", optional
	p.FilterExpr, e = GetString(j, "FilterExpr", false)
	if e != nil {
		return e
	}
	// Parse "Delimiters", optional
	p.Delimiters, e = GetString(j, "Delimiters", false)
	if e != nil {
		return e
	}
	// Parse "Fields", optional
	if i := j.Get("Fields").Interface(); i != nil {
		var ok bool
		if p.Fields, ok = i.([]PeckField); !ok {
			return errors.New("Fields format error")
		}
	}

	return nil
}
