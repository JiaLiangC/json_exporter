package exporter

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/creasty/defaults"
	"github.com/emirpasic/gods/lists/singlylinkedlist"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path"
	"reflect"
	"strconv"
	"strings"
	"sync"
)

type Config struct {
	ListenAddr   string            `yaml:"listenAddr"`
	WhiteListDir string            `yaml:"whiteListDir"`
	Components   []ComponentOption `yaml:"Components"`
}

type ComponentOption struct {
	ProcessName           string `yaml:"processName"`
	Port                  int    `yaml:"port"`
	Name                  string `yaml:"name"`
	WhiteListDir          string `yaml:"whiteListDir"`
	AllowRecursiveParse   bool   `default:"false" yaml:"allowRecursiveParse"`
	AllowMetricsWhiteList bool   `default:"true" yaml:"allowMetricsWhiteList"`
	JmxSuffix             string `default:"/jmx" yaml:"jmxUrlSuffix"`
}

type Component struct {
	ComponentOption
	promMetricsDesc  map[string]*prometheus.Desc
	metricsWhiteList singlylinkedlist.List
	logger           log.Logger
}

type Exporter struct {
	components *singlylinkedlist.List
	sgMutex    sync.Mutex
	sgWaitCh   chan struct{}
	sgChans    []chan<- prometheus.Metric
	logger     log.Logger
}

type MetricsData struct {
	value     float64
	labelPair map[string]string
	promDesc  *prometheus.Desc
}

func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	e.sgMutex.Lock()
	e.sgChans = append(e.sgChans, ch)
	// Safe to compare length since we own the Lock
	if len(e.sgChans) == 1 {
		e.sgWaitCh = make(chan struct{})
		go e.collectChans(e.sgWaitCh)
	} else {
		level.Info(e.logger).Log("info", "concurrent calls detected, waiting for first to finish")
	}
	// Put in another variable to ensure not overwriting it in another Collect once we wait
	waiter := e.sgWaitCh
	e.sgMutex.Unlock()
	// Released lock, we have insurance that our chan will be part of the collectChan slice
	<-waiter
}

func (e *Exporter) collectChans(quit chan struct{}) {
	original := make(chan prometheus.Metric)
	container := make([]prometheus.Metric, 0, 100)
	go func() {
		for metric := range original {
			container = append(container, metric)
		}
	}()
	e.collectMetrics(original)
	close(original)
	e.sgMutex.Lock()
	for _, ch := range e.sgChans {
		for _, metric := range container {
			ch <- metric
		}
	}
	e.sgChans = e.sgChans[:0]
	close(quit)
	e.sgMutex.Unlock()
}

func (e *Exporter) collectMetrics(ch chan<- prometheus.Metric) {
	getLabelValues := func(m map[string]string) []string {
		values := make([]string, 0, len(m))
		for _, value := range m {
			values = append(values, value)
		}
		return values
	}

	var wg = sync.WaitGroup{}
	componentCollect := func(component *Component) {
		defer wg.Done()
		if !component.isProcessExisted() {
			return
		}
		level.Debug(e.logger).Log("msg", "getting metrics  data from url", "url", component.composeMetricUrl())
		data, getDataErr := component.getData(component.composeMetricUrl())
		if getDataErr != nil {
			level.Error(e.logger).Log("msg", "get metrics  data from url error", "url", component.composeMetricUrl(), "error", getDataErr)
			return
		}
		res, fetchDataErr := component.fetchData(data)
		if fetchDataErr != nil {
			level.Error(e.logger).Log("msg", "err in fetchData: ", "error", fetchDataErr)
			return
		}
		//	 update metrics value,if not exist then register it
		for _, metricsData := range res {
			ch <- prometheus.MustNewConstMetric(metricsData.promDesc, prometheus.GaugeValue, metricsData.value, getLabelValues(metricsData.labelPair)...)
		}
	}

	if e.components.Size() == 0 {
		return
	}
	for iter := e.components.Iterator(); iter.Next(); {
		wg.Add(1)
		go componentCollect(iter.Value().(*Component))
	}
	wg.Wait()

}

func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	if e.components.Size() == 0 {
		return
	}
	iter := e.components.Iterator()
	for iter.Next() {
		component := iter.Value().(*Component)
		if !component.isProcessExisted() {
			continue
		}
		pdesc := component.getPromMetricsDesc()
		if len(pdesc) == 0 {
			continue
		}
		for _, metricDesc := range pdesc {
			ch <- metricDesc
		}
	}
}

func getHostName() string {
	hostName, _ := os.Hostname()
	return hostName
}

func (e *Component) composeMetricUrl() string {
	url := fmt.Sprintf("http://%s:%d%s", getHostName(), e.Port, e.JmxSuffix)
	return url
}
func (e *Component) isProcessExisted() bool {
	cmdStr := fmt.Sprintf("ps -ef |grep %s |grep -v grep", e.ProcessName)
	cmd := exec.Command("/bin/sh", "-c", cmdStr)
	res, _ := cmd.Output()
	if len(string(res)) > 0 {
		return true
	}
	return false
}

//register 后存入hash，之后取出 set value
func (e *Component) getData(requestURL string) (map[string]interface{}, error) {
	resp, err := http.Get(requestURL)
	if err != nil {
		return nil, errors.New("get data from " + requestURL + " failed")
	}
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.New("read data from " + requestURL + " json failed")
	}
	var f = make(map[string]interface{})
	err = json.Unmarshal(data, &f)
	if err != nil {
		return nil, errors.New("parse json failed")
	}
	return f, nil
}

func (e *Component) generateLabelPairs(nameDataMap map[string]interface{}) map[string]string {
	labels := make(map[string]string)
	if dictName, ok := nameDataMap["name"]; ok {
		dictNameStr := dictName.(string)
		if len(dictNameStr) > 0 {
			labels["name"] = dictNameStr
		}
	}
	return labels
}

func (e *Component) filterMetricsValue(clearedMetricsKey string, metricsValue interface{}) (MetricsData, error) {
	whiteList, getWhitelistErr := e.getWhitelist()
	if getWhitelistErr != nil {
		return MetricsData{}, getWhitelistErr
	}
	metricsData := MetricsData{}
	strValue := fmt.Sprint(metricsValue)
	isInWhiteList := whiteList.Any(func(index int, value interface{}) bool {
		return strings.Compare(clearedMetricsKey, value.(string)) == 0
	})

	if e.AllowMetricsWhiteList && !isInWhiteList {
		return MetricsData{}, errors.New("not in WhiteList")
	}

	floatValue, err := strconv.ParseFloat(strValue, 64)
	if err != nil {
		return MetricsData{}, errors.New("value is not in numeric format")
	}
	metricsData.value = floatValue
	return metricsData, nil
}

func (e *Component) fetchData(data map[string]interface{}) (map[string]MetricsData, error) {
	dataList := make(map[string]MetricsData)
	var recursiveFetch func(data interface{}, hierarchy []string) interface{}

	recursiveFetch = func(data interface{}, hierarchy []string) interface{} {
		switch dataType := reflect.ValueOf(data).Kind(); dataType {
		case reflect.Map:
			labels := e.generateLabelPairs(data.(map[string]interface{}))
			for metricsKey, metricsValue := range data.(map[string]interface{}) {
				clearedMetricsKey := metricsKeyClear(metricsKey)
				valueKind := reflect.ValueOf(metricsValue).Kind()
				//开启递归采集时 才会递归进入Map，否则只处理单层map
				if valueKind == reflect.Map || valueKind == reflect.Slice {
					hierarchy = append(hierarchy, clearedMetricsKey)
					if e.AllowRecursiveParse {
						level.Debug(e.logger).Log("msg", "recursive fetch start")
						return recursiveFetch(metricsValue, hierarchy)
					}
				} else {
					keyArr := append(hierarchy, clearedMetricsKey)
					var finalKey string
					if len(hierarchy) == 0 {
						finalKey = clearedMetricsKey
					} else {
						finalKey = strings.Join(keyArr, "_")
						level.Debug(e.logger).Log("msg", "finalKey in recursive ", "key", finalKey)
					}

					metricsData, filterErr := e.filterMetricsValue(finalKey, metricsValue)
					if filterErr == nil {
						metricsData.promDesc = e.getOrCreatePromDesc(finalKey, labels)
						metricsData.labelPair = labels
						dataList[finalKey] = metricsData
					}
				}
			}
		case reflect.Slice:
			for _, item := range data.([]interface{}) {
				itemKind := reflect.ValueOf(item).Kind()
				if itemKind == reflect.Map || itemKind == reflect.Slice {
					return recursiveFetch(item, hierarchy)
				}
			}
		default:
		}

		return nil
	}

	treePath := make([]string, 0)
	if value, ok := data["beans"]; ok {
		var nameList = value.([]interface{})
		for _, nameData := range nameList {
			nameDataMap := nameData.(map[string]interface{})
			recursiveFetch(nameDataMap, treePath)
		}
	} else {
		recursiveFetch(data, treePath)
	}
	return dataList, nil
}
func (e *Component) getOrCreatePromDesc(metricsKey string, labels map[string]string) *prometheus.Desc {
	getLabelVariables := func(mp map[string]string) []string {
		keys := make([]string, 0, len(mp))
		for k := range mp {
			keys = append(keys, k)
		}
		return keys
	}

	if promDesc, ok := e.promMetricsDesc[metricsKey]; !ok {
		hostName, _ := os.Hostname()
		constLabelPair := map[string]string{"hostname": hostName, "component": e.Name}
		promDesc = prometheus.NewDesc(
			prometheus.BuildFQName("", "", metricsKey),
			"description", getLabelVariables(labels), constLabelPair)
		e.promMetricsDesc[metricsKey] = promDesc
	}
	return e.promMetricsDesc[metricsKey]
}

func (e *Component) getPromMetricsDesc() map[string]*prometheus.Desc {
	return e.promMetricsDesc
}

func (e *Component) getWhitelist() (singlylinkedlist.List, error) {
	return e.metricsWhiteList, nil
}

func (e *Component) initialize() error {
	if e.metricsWhiteList.Size() == 0 || len(e.promMetricsDesc) == 0 {
		if e.promMetricsDesc == nil {
			e.promMetricsDesc = make(map[string]*prometheus.Desc)
		}
		fileName := e.Name + ".json"
		fnameAbsPath := path.Join(e.WhiteListDir, fileName)
		level.Debug(e.logger).Log("msg", "read json file", "path", fnameAbsPath)
		fp, err := os.Open(fnameAbsPath)
		if err != nil {
			level.Error(e.logger).Log("msg", "open json file error", "path", fnameAbsPath, "error", err)
			return err
		}
		defer fp.Close()
		bytes, err := ioutil.ReadAll(fp)
		if err != nil {
			level.Error(e.logger).Log("msg", "read json file error", "path", fnameAbsPath, "error", err)
			return err
		}

		var metricsWhiteListJsonMap map[string]string
		metricsWhiteListJsonMap = make(map[string]string)
		err = json.Unmarshal(bytes, &metricsWhiteListJsonMap)
		if err != nil {
			level.Error(e.logger).Log("msg", "Unmarshal json file error", "path", fnameAbsPath, "error", err)
			return err
		}
		for metricsKey, _ := range metricsWhiteListJsonMap {
			clearedMetricsKey := metricsKeyClear(metricsKey)
			e.metricsWhiteList.Add(clearedMetricsKey)
		}
	}
	level.Debug(e.logger).Log("whitelist", e.metricsWhiteList.String())
	return nil
}

func metricsKeyClear(metricsKey string) string {
	if strings.IndexByte(metricsKey, '.') != -1 {
		metricsKey = strings.ReplaceAll(metricsKey, ".", "_")
	}

	if strings.IndexByte(metricsKey, '-') != -1 {
		metricsKey = strings.ReplaceAll(metricsKey, "-", "_")
	}
	return metricsKey
}

func NewExporter(logger log.Logger, config *Config) (*Exporter, error) {
	e := Exporter{
		components: singlylinkedlist.New(),
		sgMutex:    sync.Mutex{},
		sgWaitCh:   nil,
		sgChans:    []chan<- prometheus.Metric{},
		logger:     logger,
	}

	for _, componentOption := range config.Components {
		if len(componentOption.WhiteListDir) == 0 {
			componentOption.WhiteListDir = config.WhiteListDir
		}
		c := &Component{logger: logger, ComponentOption: componentOption}
		c.initialize()
		e.components.Add(c)
	}
	return &e, nil
}

func (s *Config) UnmarshalYAML(unmarshal func(interface{}) error) error {
	defaults.Set(s)
	type plain Config
	if err := unmarshal((*plain)(s)); err != nil {
		return err
	}
	return nil
}

func (s *ComponentOption) UnmarshalYAML(unmarshal func(interface{}) error) error {
	defaults.Set(s)
	type plain ComponentOption
	if err := unmarshal((*plain)(s)); err != nil {
		return err
	}
	return nil
}
