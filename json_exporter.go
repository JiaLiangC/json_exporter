package main

import (
	"github.com/JiaLiangC/json_exporter/exporter"
	klog "github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"net/http"
	"os"
	"path"
)

func configLoad(logger log.Logger, path string) *exporter.Config {
	r, err := os.Open(path)
	defer r.Close()
	if err != nil {
		level.Error(logger).Log("msg", "can't open config file", "error", err)
	}
	data, err := ioutil.ReadAll(r)
	if err != nil {
		level.Error(logger).Log("msg", "can't read config file", "error", err)
	}
	config := new(exporter.Config)
	yaml.Unmarshal(data, config)
	if err != nil {
		level.Error(logger).Log("msg", "cannot unmarshal yaml data ", "error", err)
	}
	return config
}

func setupExporter() {
	w := klog.NewSyncWriter(os.Stdout)
	logger := klog.NewLogfmtLogger(w)
	dir, err := os.Getwd()
	if err != nil {
		level.Error(logger).Log("msg", "Error handle / request", "error", err)
	}

	config := configLoad(logger, path.Join(dir, "conf/conf.yaml"))
	exporter, _ := exporter.NewExporter(logger, config)

	prometheus.MustRegister(exporter)
	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, err := w.Write([]byte(`<html>
	        <head><title>Hadoop Exporter</title></head>
	        <body>
	        <h1>Hadoop Exporter</h1>
	        <p><a href='/metrics'>Metrics</a></p>
	        </body>
	        </html>`))
		if err != nil {
			level.Error(logger).Log("msg", "Error handle / request", "error", err)
		}
	})
	httpErr := http.ListenAndServe(config.ListenAddr, nil)

	if httpErr != nil {
		level.Error(logger).Log("msg", "http server start error:", "error", httpErr)
	}
	level.Info(logger).Log("listen_addr", config.ListenAddr)
}

func main() {
	setupExporter()
}
