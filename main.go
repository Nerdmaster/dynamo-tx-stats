package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Transaction struct {
	Address       string  `json:"address"`
	Category      string  `json:"category"`
	Amount        float64 `json:"amount"`
	Label         string  `json:"label"`
	Confirmations int64   `json:"confirmations"`
	Generated     bool    `json:"generated"`
	Blockhash     string  `json:"blockhash"`
	Blockheight   int64   `json:"blockheight"`
	Blockindex    int64   `json:"blockindex"`
	Blocktime     int64   `json:"blocktime"`
	TXID          string  `json:"txid"`
	dt            time.Time
	Time          int64 `json:"time"`
	TimeReceived  int64 `json:"timereceived"`
}

type StatData struct {
	firstBlock int64
	lastBlock  int64
	duration   time.Duration
	blocks     int64
	coins      float64
}

func (s *StatData) record(tx *Transaction) {
	if s.firstBlock == 0 || tx.Blockheight < s.firstBlock {
		s.firstBlock = tx.Blockheight
	}
	if tx.Blockheight > s.lastBlock {
		s.lastBlock = tx.Blockheight
	}
	s.coins += tx.Amount
	s.blocks++
}

func (s *StatData) roughPercent() float64 {
	if s.blocks == 0 {
		return 0
	}
	return 100.0 * (float64(s.blocks) / float64(s.lastBlock-s.firstBlock))
}

func getDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
}

func usage(message string) {
	fmt.Fprintln(os.Stderr, message)
	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stderr, "Usage: %s <url> <username> <password> <report days> <Wallet Name(s)...>\n", os.Args[0])
	os.Exit(1)
}

func fetchTX(u *url.URL) []*Transaction {
	var resp struct {
		Results []*Transaction `json:"result"`
	}

	var data = bytes.NewBufferString(`{"jsonrpc":"1.0","id":"curltest","method":"listtransactions","params":["*", 10000, 0]}`)
	var err = doPost(u, data, &resp)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to POST to URL %q: %s", u.String(), err)
		os.Exit(2)
	}

	return resp.Results
}

func main() {
	if len(os.Args) < 6 {
		usage("Not enough args")
	}

	var urlString, user, pass, rdstr = os.Args[1], os.Args[2], os.Args[3], os.Args[4]
	var reportDays, _ = strconv.Atoi(rdstr)
	if reportDays == 0 {
		usage(fmt.Sprintf("Invalid reporting days value %q", rdstr))
	}
	if reportDays < 2 {
		usage("Reporting days must be at least 2")
	}
	var wallets = os.Args[5:]

	var u, err = url.Parse(urlString)
	if err != nil {
		usage(fmt.Sprintf("Invalid URL %q: %s", urlString, err))
	}

	u.User = url.UserPassword(user, pass)
	var txList []*Transaction
	for _, w := range wallets {
		u.Path = "/wallet/" + w
		txList = append(txList, fetchTX(u)...)
	}

	fmt.Printf("%d transactions (wallet(s): %s)\n", len(txList), strings.Join(wallets, ", "))
	var reportStats StatData
	var dailyStats = make([]StatData, reportDays)
	var hourlyStats = make([]StatData, 24)
	var now = time.Now()
	var nowDay = getDay(now)
	var daysAgo = time.Duration(reportDays-1) * time.Hour * -24
	var beginReport = nowDay.Add(daysAgo)

	for _, tx := range txList {
		tx.dt = time.Unix(tx.TimeReceived, 0)

		if !tx.Generated {
			continue
		}
		if tx.Confirmations < 6 {
			continue
		}

		if tx.dt.Before(beginReport) {
			continue
		}

		reportStats.record(tx)

		var dayIndex = int(tx.dt.Sub(beginReport) / time.Hour / 24)
		dailyStats[dayIndex].record(tx)

		if dayIndex == reportDays-1 {
			hourlyStats[tx.dt.Hour()].record(tx)
		}
	}

	var first = txList[0]
	fmt.Printf("First tx was recorded at %s\n", first.dt.Format("2006-01-02 15:04:05"))
	var total = reportStats.coins
	fmt.Printf("Report period total: %0.2f\n", total)
	fmt.Printf("Daily average: %0.2f\n", total/float64(reportDays))
	fmt.Printf("Hourly average: %0.2f\n", total/float64(reportDays)/24.0)
	fmt.Printf("Rough Block Win Percent: %0.4f%%\n", reportStats.roughPercent())

	for i := 0; i < reportDays; i++ {
		var projection = ""
		var coins = dailyStats[i].coins
		var hours = 24.0
		var when = beginReport.Add(time.Hour * 24 * time.Duration(i)).Format("2006-01-02")
		if i == reportDays-1 {
			hours = float64(now.Hour()) + float64(now.Minute())/60.0
			projection = fmt.Sprintf(" (~ %0.2f expected)", coins/hours*24)
		}
		fmt.Printf("%s:\t\t\t%8.2f\t\t%0.2f/h\t\tWin%%: %0.4f%%%s\n", when, coins, coins/hours, dailyStats[i].roughPercent(), projection)
	}

	for i := 0; i <= now.Hour(); i++ {
		var projection = ""
		var coins = hourlyStats[i].coins
		var minutes = 60.0
		var when = fmt.Sprintf("%s/%02d", getDay(now).Format("2006-01-02"), i)
		if i == now.Hour() {
			minutes = float64(now.Minute()) + float64(now.Second())/60
			projection = fmt.Sprintf("(~ %0.2f expected)", coins/minutes*60)
		}
		fmt.Printf("- %s:\t\t%8.2f\t\t%0.2f/m\t%s\n", when, coins, coins/minutes, projection)
	}
}

func doPost(u *url.URL, data io.Reader, resp interface{}) error {
	var r, err = http.Post(u.String(), "text/plain", data)
	if err != nil {
		return err
	}
	var body []byte
	body, err = io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	defer r.Body.Close()

	err = json.Unmarshal(body, &resp)
	if err != nil {
		return err
	}

	return nil
}
