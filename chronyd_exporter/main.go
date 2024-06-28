package main

import (
	"log"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// ChronydStatusCollector collects metrics about chronyd status
type ChronydStatusCollector struct {
	statusDesc     *prometheus.Desc
	leapStatusDesc *prometheus.Desc
	timeDesc       *prometheus.Desc
}

func NewChronydStatusCollector() *ChronydStatusCollector {
	return &ChronydStatusCollector{
		statusDesc:     prometheus.NewDesc("chronyd_status", "Chronyd status (1 = running, 0 = not running)", nil, nil),
		timeDesc:       prometheus.NewDesc("current_time", "Current host time in Unix timestamp", nil, nil),
		leapStatusDesc: prometheus.NewDesc("chronyd_leap_status", "Chronyd leap status (0 = Normal, 1 = Insert second, 2 = Delete second, 3 = Not synchronised, 4 = Unkonw)", nil, nil),
	}
}

func (collector *ChronydStatusCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- collector.statusDesc
	ch <- collector.timeDesc
	ch <- collector.leapStatusDesc
}

func (collector *ChronydStatusCollector) Collect(ch chan<- prometheus.Metric) {
	// Check chronyd status
	status := checkChronydStatus()

	// Get current time
	currentTime := float64(time.Now().Unix())

	// Check chronyd leap status
	leapStatus := checkChronycLeapStatus()

	// Send metrics
	ch <- prometheus.MustNewConstMetric(collector.statusDesc, prometheus.GaugeValue, status)
	ch <- prometheus.MustNewConstMetric(collector.timeDesc, prometheus.GaugeValue, currentTime)
	ch <- prometheus.MustNewConstMetric(collector.leapStatusDesc, prometheus.GaugeValue, leapStatus)
}

func checkChronydStatus() float64 {
	// Execute system command to check chronyd status
	out, err := exec.Command("systemctl", "is-active", "chronyd").Output()
	if err != nil {
		log.Printf("Error checking chronyd status: %v", err)
		return 0
	}
	status := strings.TrimSpace(string(out))
	if status == "active" {
		return 1
	}
	return 0
}

func checkChronycLeapStatus() float64 {
	out, err := exec.Command("chronyc", "tracking").Output()
	if err != nil {
		log.Printf("Error checking chronyc tracking leap status: %v\n", err)
		return -1
	}
	if strings.Contains(string(out), "Leap status     : Normal") {
		return 0
	} else if strings.Contains(string(out), "Leap status     : Insert second") {
		return 1
	} else if strings.Contains(string(out), "Leap status     : Delete second") {
		return 2
	} else if strings.Contains(string(out), "Leap status     : Not synchronised") {
		return 3
	} else {
		return 4
	}
}

func main() {
	// Create a new instance of the ChronydStatusCollector
	collector := NewChronydStatusCollector()
	prometheus.MustRegister(collector)

	// Expose the registered metrics via HTTP
	http.Handle("/metrics", promhttp.Handler())
	log.Println("Beginning to serve on port :9105")
	log.Fatal(http.ListenAndServe(":9105", nil))
}
