package logpeck

import (
	"bytes"
	"encoding/json"
	"errors"
	log "github.com/Sirupsen/logrus"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"strings"
	"sync"
	"time"
)

type ElasticSearchConfig struct {
	Hosts   []string               `json:"Hosts"`
	Index   string                 `json:"Index"`
	Type    string                 `json:"Type"`
	Mapping map[string]interface{} `json:"Mapping"`
}

type ElasticSearchSender struct {
	config        ElasticSearchConfig
	mu            sync.Mutex
	lastIndexName string
}

func NewElasticSearchSenderConfig(jbyte []byte) (ElasticSearchConfig, error) {
	elasticSearchConfig := ElasticSearchConfig{}
	err := json.Unmarshal(jbyte, &elasticSearchConfig)
	if err != nil {
		return elasticSearchConfig, err
	}
	log.Infof("[NewElasticSearchSenderConfig]ElasticSearchConfig: %v", elasticSearchConfig)
	return elasticSearchConfig, nil
}

func NewElasticSearchSender(senderConfig *SenderConfig) (*ElasticSearchSender, error) {
	sender := ElasticSearchSender{}
	config, ok := senderConfig.Config.(ElasticSearchConfig)
	if !ok {
		return &sender, errors.New("New ElasticSearchSender error ")
	}
	sender = ElasticSearchSender{
		config: config,
	}
	return &sender, nil
}

func HttpCall(method, url string, bodyString string) {
	body := ioutil.NopCloser(bytes.NewBuffer([]byte(bodyString)))

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		log.Infof("[Sender] New request error, err[%s]", err)
	}
	client := &http.Client{Timeout: time.Duration(500) * time.Millisecond}
	resp, err := client.Do(req)
	if err != nil {
		log.Infof("[Sender] Put error, err[%s]", err)
	} else {
		resp_str, _ := httputil.DumpResponse(resp, true)
		log.Infof("[Sender] Response %s", resp_str)
	}
}

func (p *ElasticSearchSender) GetIndexName() (indexName string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	prototype := p.config.Index
	l, r := "%{+", "}"
	if !strings.Contains(prototype, l) || !strings.Contains(prototype, r) {
		indexName = prototype
	} else {
		lIndex := strings.Index(prototype, l)
		rIndex := strings.Index(prototype, r)
		format := prototype[lIndex+len(l) : rIndex]
		timeStr := time.Now().Format(format)
		indexName = prototype[:lIndex] + timeStr + prototype[rIndex+1:]
	}

	if indexName != p.lastIndexName {
		p.lastIndexName = indexName
		p.InitMapping()
	}

	return indexName
}

func (p *ElasticSearchSender) InitMapping() error {
	host, err := SelectRandom(p.config.Hosts)
	if err != nil {
		return err
	}
	uri := "http://" + host + "/" + p.lastIndexName
	typeUri := uri + "/_mappings/" + p.config.Type

	// Try init index mapping
	// indexMapping := `{"mappings":` + p.config.Mapping + `}`
	indexMapping := map[string]interface{}{
		"mappings": p.config.Mapping,
	}
	raw_data, err := json.Marshal(indexMapping)
	if p.config.Mapping == nil {
		raw_data = []byte(`{"mappings":{}}`)
	}
	log.Infof("[Sender] Init ElasticSearch mapping %s %s ", uri, string(raw_data[:]))
	HttpCall(http.MethodPut, uri, string(raw_data[:]))

	// Try init Timestamp Field mapping
	propString := `{"properties":{"Timestamp":{"type":"date","format":"epoch_millis"}}}`
	log.Infof("[Sender] Init ElasticSearch mapping %s %s ", uri, propString)
	HttpCall(http.MethodPut, typeUri, propString)

	return nil
}

func (p *ElasticSearchSender) Start() error {
	return nil
}

func (p *ElasticSearchSender) Stop() error {
	return nil
}

func (p *ElasticSearchSender) Send(fields map[string]interface{}) {
	defer LogExecTime(time.Now(), "Sender")
	data := map[string]interface{}{
		"Host":      GetHost(),
		"Timestamp": time.Now().UnixNano() / 1000000,
	}
	for k, v := range fields {
		data[k] = v
	}
	raw_data, err := json.Marshal(data)
	if err != nil {
		panic(err)
	}
	host, err := SelectRandom(p.config.Hosts)
	if err != nil {
		log.Debugf("[Sender] ElasticSearch Host error [%v] ", err)
		return
	}
	uri := "http://" + host + "/" + p.GetIndexName() + "/" + p.config.Type
	log.Debugf("[Sender] Post ElasticSearch %s content [%s] ", uri, raw_data)
	body := ioutil.NopCloser(bytes.NewBuffer(raw_data))
	resp, err := http.Post(uri, "application/json", body)
	if err != nil {
		log.Infof("[Sender] Post error, err[%s]", err)
	} else {
		resp_str, _ := httputil.DumpResponse(resp, true)
		log.Debugf("[Sender] Response %s", resp_str)
	}
}
