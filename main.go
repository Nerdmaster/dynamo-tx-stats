package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

func usage(message string) {
	fmt.Fprintln(os.Stderr, message)
	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stderr, "Usage: %s <url> <username> <password> <Wallet Name(s)...>\n", os.Args[0])
	os.Exit(1)
}

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
	if len(os.Args) < 5 {
		usage("Not enough args")
	}

	var urlString, user, pass = os.Args[1], os.Args[2], os.Args[3]
	var wallets = os.Args[4:]

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
	var stats = make(map[string]float64)
	var now = time.Now()
	var today = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	const reportDays = 10
	var beginReport = today.Add(time.Hour * 24 * -reportDays)
	for _, tx := range txList {
		tx.dt = time.Unix(tx.TimeReceived, 0)

		if !tx.Generated {
			continue
		}

		if tx.dt.Before(beginReport) {
			continue
		}

		stats["_reporting total"] += tx.Amount

		stats[tx.dt.Format("2006-01-02")] += tx.Amount
		if tx.dt.Day() == now.Day() {
			stats[tx.dt.Format("2006-01-02/15")] += tx.Amount
		}
	}

	var first = txList[0]
	fmt.Printf("First tx was recorded at %s\n", first.dt.Format("2006-01-02 15:04:05"))
	var total = stats["_reporting total"]
	fmt.Printf("Report period total: %0.2f\nDaily average: %0.2f\nHourly average: %0.2f\n", total, total/reportDays, total/reportDays/24.0)
	var keys []string
	for k := range stats {
		if k[0] == '_' {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		var projection = ""
		if len(k) > 10 {
			var minutes = 60.0
			if k == now.Format("2006-01-02/15") {
				minutes = float64(now.Minute()) + float64(now.Second())/60
				projection = fmt.Sprintf("(~ %0.2f expected)", stats[k]/minutes*60)
			}
			fmt.Printf("- %s:\t\t%8.2f\t\t%0.2f/m\t%s\n", k, stats[k], stats[k]/minutes, projection)
		} else {
			var hours = 24.0
			if k == now.Format("2006-01-02") {
				hours = float64(now.Hour()) + float64(now.Minute())/60.0
				projection = fmt.Sprintf("(~ %0.2f expected)", stats[k]/hours*24)
			}
			fmt.Printf("%s:\t\t\t%8.2f\t\t%0.2f/h\t%s\n", k, stats[k], stats[k]/hours, projection)
		}
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
