package main

import (
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/nrdcg/namesilo"
)

var api string

func main() {

	var domain string
	var hostname string
	var token string
	var historyFilepath string

	flag.StringVar(&domain, "domain", "", "namesilo domain")
	flag.StringVar(&hostname, "hostname", "", "namesilo dns hostname")
	flag.StringVar(&token, "token", "", "namesilo api token")
	flag.StringVar(&api,"api","https://api.ipify.org","ipv4 api")
	flag.Parse()

	switch {
	case domain == "":
		slog.Error("domain is empty")
		return
	case hostname == "":
		slog.Error("hostname is empty")
		return
	case token == "":
		slog.Error("token is empty")
		return
	}

	switch runtime.GOOS {
	case "linux":
		historyFilepath = filepath.Join("/var", "namesilo-ddns-history")
	case "windows":
		historyFilepath = filepath.Join(os.Getenv("TEMP"), "namesilo-ddns-history")
	}

	// get history ip if file exist, otherwise history ip is empty
	var history string
	_, err := os.Stat(historyFilepath)
	if !os.IsNotExist(err) {
		byt, err := os.ReadFile(historyFilepath)
		if err != nil {
			slog.Error("get history ip error", "message", err)
			return
		}
		history = string(byt)
	}

	// get ip
	ip, err := getIP()
	if err != nil {
		slog.Error("get ip error", "message", err)
		return
	}

	// IP has not changed
	if ip == history {
		slog.Info("ip has not changed")
		return
	}

	client, err := getNamesiloClient(token)
	if err != nil {
		slog.Error("get namesilo client error", "message", err)
		return
	}

	// get dns record
	records, err := client.DnsListRecords(&namesilo.DnsListRecordsParams{Domain: domain})
	if err != nil {
		slog.Error("get namesilo records error", "message", err)
		return
	}

	if records.Reply.Code != "300" {
		slog.Error("get namesilo records error", "code", records.Reply.Code, "detail", records.Reply.Detail)
		return
	}

	var record namesilo.ResourceRecord
	for _, item := range records.Reply.ResourceRecord {
		// find record
		if fmt.Sprintf("%s.%s", hostname, domain) == item.Host && item.Type == "A" {
			record = item
			break
		}
	}

	// record not found, add
	if record.RecordID == "" {

		if _, err = client.DnsAddRecord(&namesilo.DnsAddRecordParams{
			Domain:   domain,
			Type:     "A",
			Host:     hostname,
			Value:    ip,
			Distance: 0,
			TTL:      7207,
		}); err != nil {
			slog.Error("add dns record error", "message", err)
			return
		}

		if records.Reply.Code != "300" {
			slog.Error("add namesilo records error", "code", records.Reply.Code, "detail", records.Reply.Detail)
			return
		}

		slog.Info("add record success", "host", fmt.Sprintf("%s.%s", hostname, domain), "value", ip)
	} else {
		// record found, update

		distance, err := strconv.Atoi(record.Distance)
		if err != nil {
			slog.Error("distance string to integer error", "message", err)
			return
		}
		ttl, err := strconv.Atoi(record.TTL)
		if err != nil {
			slog.Error("ttl string to integer error", "message", err)
			return
		}

		if _, err = client.DnsUpdateRecord(&namesilo.DnsUpdateRecordParams{
			Domain:   domain,
			ID:       record.RecordID,
			Host:     hostname,
			Value:    ip,
			Distance: distance,
			TTL:      ttl,
		}); err != nil {
			slog.Error("update dns record error", "message", err)
			return
		}

		if records.Reply.Code != "300" {
			slog.Error("update namesilo records error", "code", records.Reply.Code, "detail", records.Reply.Detail)
			return
		}

		slog.Info("update record success", "host", fmt.Sprintf("%s.%s", hostname, domain), "value", ip)
	}

	// save ip
	err = os.WriteFile(historyFilepath, []byte(ip), 0644)
	if err != nil {
		slog.Error("ip write to temp file error", "message", err)
		return
	}

}

func getIP() (string, error) {
	resp, err := http.Get(api)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("http response status code [%d] is not 200", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

func getNamesiloClient(token string) (client *namesilo.Client, err error) {
	transport, err := namesilo.NewTokenTransport(token)
	if err != nil {
		return
	}

	client = namesilo.NewClient(transport.Client())
	return
}
