package main

import (
	"flag"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/net/publicsuffix"
)

var (
	baseUrl = flag.String("url", "http://192.168.100.1", "base URL to modem")
	user    = flag.String("user", "admin", "username to login")
	pass    = flag.String("pass", "password", "password to login")

	channelInfo = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "netgear_channel_info",
	}, []string{"channel", "lock_status", "modulation", "channel_id", "frequency"})

	power = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "netgear_power",
		Help: "Power in dBmV",
	}, []string{"channel"})

	snrmer = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "netgear_snrmer",
		Help: "SNR/MER in dB",
	}, []string{"channel"})

	unerroredCodewords = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "netgear_unerrored_codewords",
		Help: "number of unerrored codewords",
	}, []string{"channel"})

	correctableCodewords = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "netgear_correctable_codewords",
		Help: "number of correctable codewords",
	}, []string{"channel"})

	uncorrectableCodewords = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "netgear_uncorrectable_codewords",
		Help: "number of uncorrectable codewords",
	}, []string{"channel"})
)

const (
	loginPath   = "/GenieLogin.asp"
	loginAction = "/goform/GenieLogin"
	statusPath  = "/DocsisStatus.asp"
)

func main() {
	prometheus.MustRegister(
		channelInfo,
		power,
		snrmer,
		unerroredCodewords,
		correctableCodewords,
		uncorrectableCodewords,
	)

	http.Handle("/metrics", promhttp.Handler())
	go http.ListenAndServe(":9090", nil)

	err := getMetrics()
	if err != nil {
		panic(err)
	}

	for range time.Tick(time.Minute) {
		err = getMetrics()
		if err != nil {
			panic(err)
		}
	}
}

func getMetrics() error {
	jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	if err != nil {
		return err
	}

	client := &http.Client{
		Jar: jar,
	}

	resp, err := client.Get(*baseUrl + loginPath)
	if err != nil {
		return err
	}

	doc, err := goquery.NewDocumentFromResponse(resp)
	if err != nil {
		return err
	}

	webToken, _ := doc.Find("input[name=webToken]").Attr("value")

	resp, err = client.PostForm(*baseUrl+loginAction, url.Values{
		"webToken":      []string{webToken},
		"loginUsername": []string{*user},
		"loginPassword": []string{*pass},
	})
	if err != nil {
		return err
	}

	resp.Body.Close()

	resp, err = client.Get(*baseUrl + statusPath)
	doc, err = goquery.NewDocumentFromResponse(resp)
	if err != nil {
		return err
	}

	tableRows := doc.Find(".in-frame-table table tr")
	tableRows.Each(func(i int, s *goquery.Selection) {
		cells := s.Find("td")

		if i > 0 {
			if len(cells.Nodes) != 10 {
				return
			}

			Channel := cells.First()
			LockStatus := Channel.Next()
			Modulation := LockStatus.Next()
			ChannelID := Modulation.Next()
			Frequency := ChannelID.Next()

			Power := Frequency.Next()
			SNRMER := Power.Next()
			UnerroredCodewords := SNRMER.Next()
			CorrectableCodewords := UnerroredCodewords.Next()
			UncorrectableCodewords := CorrectableCodewords.Next()
			log.Printf(
				"%s - %s - %s - %s - %s - %s - %s - %s - %s - %s",
				Channel.Text(),
				LockStatus.Text(),
				Modulation.Text(),
				ChannelID.Text(),
				Frequency.Text(),
				Power.Text(),
				SNRMER.Text(),
				UnerroredCodewords.Text(),
				CorrectableCodewords.Text(),
				UncorrectableCodewords.Text(),
			)

			channelInfo.WithLabelValues(
				Channel.Text(),
				LockStatus.Text(),
				Modulation.Text(),
				ChannelID.Text(),
				Frequency.Text(),
			).Set(1.0)

			p, err := strconv.ParseFloat(strings.TrimSpace(strings.Replace(Power.Text(), "dBmV", "", -1)), 64)
			if err != nil {
				log.Printf("Error parsing power: %v", err)
			} else {
				power.WithLabelValues(Channel.Text()).Set(p)
			}

			s, err := strconv.ParseFloat(strings.TrimSpace(strings.Replace(SNRMER.Text(), "dB", "", -1)), 64)
			if err != nil {
				log.Printf("Error parsing snr/mer: %v", err)
			} else {
				snrmer.WithLabelValues(Channel.Text()).Set(s)
			}

			uec, err := strconv.ParseFloat(strings.TrimSpace(UnerroredCodewords.Text()), 64)
			if err != nil {
				log.Printf("Error parsing unerrored codewords: %v", err)
			} else {
				unerroredCodewords.WithLabelValues(Channel.Text()).Set(uec)
			}

			cc, err := strconv.ParseFloat(strings.TrimSpace(CorrectableCodewords.Text()), 64)
			if err != nil {
				log.Printf("Error parsing correctable codewords: %v", err)
			} else {
				correctableCodewords.WithLabelValues(Channel.Text()).Set(cc)
			}

			ucc, err := strconv.ParseFloat(strings.TrimSpace(UncorrectableCodewords.Text()), 64)
			if err != nil {
				log.Printf("Error parsing uncorrectable codewords: %v", err)
			} else {
				uncorrectableCodewords.WithLabelValues(Channel.Text()).Set(ucc)
			}
		}
	})

	return nil
}
