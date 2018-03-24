package main

import (
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/soniah/gosnmp"
)

const prefix = "apc_env_"

var (
	upDesc   *prometheus.Desc
	tempDesc *prometheus.Desc
	humDesc  *prometheus.Desc
)

func init() {
	l := []string{"target"}
	upDesc = prometheus.NewDesc(prefix+"up", "Scrape of target was successful", l, nil)
	l = append(l, "name", "location")
	tempDesc = prometheus.NewDesc(prefix+"temp", "Current temperature", l, nil)
	humDesc = prometheus.NewDesc(prefix+"humidity_percent", "Current humidity", l, nil)
}

type apcEnvCollector struct {
}

func (c apcEnvCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- upDesc
	ch <- tempDesc
	ch <- humDesc
}

func (c apcEnvCollector) collectTarget(target string, ch chan<- prometheus.Metric, wg *sync.WaitGroup) {
	defer wg.Done()
	snmp := &gosnmp.GoSNMP{
		Target:    target,
		Port:      161,
		Community: *snmpCommunity,
		Version:   gosnmp.Version2c,
		Timeout:   time.Duration(2) * time.Second,
	}
	err := snmp.Connect()
	if err != nil {
		log.Infof("Connect() err: %v\n", err)
		ch <- prometheus.MustNewConstMetric(upDesc, prometheus.GaugeValue, 0, target)
		return
	}
	defer snmp.Conn.Close()

	oids := []string{"1.3.6.1.4.1.318.1.1.10.4.2.3.1.3", "1.3.6.1.4.1.318.1.1.10.4.2.3.1.4",
		"1.3.6.1.4.1.318.1.1.10.4.2.3.1.5", "1.3.6.1.4.1.318.1.1.10.4.2.3.1.6"}
	var sensors []Sensor

	for i, oid := range oids {
		n := 0
		err = snmp.Walk(oid, func(pdu gosnmp.SnmpPDU) error {
			switch i {
			case 0:
				sensors = append(sensors, Sensor{Name: string(pdu.Value.([]byte))})
			case 1:
				sensors[n].Location = string(pdu.Value.([]byte))
			case 2:
				sensors[n].Temperature = float64(pdu.Value.(int))
			case 3:
				sensors[n].Humidity = float64(pdu.Value.(int))
			}
			n++
			return nil
		})
		if err != nil {
			continue
		}
	}

	for _, s := range sensors {
		ch <- prometheus.MustNewConstMetric(tempDesc, prometheus.GaugeValue, s.Temperature, target, s.Name, s.Location)
		ch <- prometheus.MustNewConstMetric(humDesc, prometheus.GaugeValue, s.Humidity, target, s.Name, s.Location)
	}

	ch <- prometheus.MustNewConstMetric(upDesc, prometheus.GaugeValue, 1, target)
}

func (c apcEnvCollector) Collect(ch chan<- prometheus.Metric) {
	targets := strings.Split(*snmpTargets, ",")
	wg := &sync.WaitGroup{}

	for _, target := range targets {
		wg.Add(1)
		go c.collectTarget(target, ch, wg)
	}

	wg.Wait()
}
