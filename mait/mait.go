package mait

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ... (Imports & Structs Same as before) ...
// URLs
const (
	BaseURL      = "http://217.182.195.194"
	LoginURL     = BaseURL + "/ints/login"
	SigninURL    = BaseURL + "/ints/signin"
	ReportsPage  = BaseURL + "/ints/agent/SMSCDRReports"
	SMSApiURL    = BaseURL + "/ints/agent/res/data_smscdr.php"
	NumberApiURL = BaseURL + "/ints/agent/res/data_smsnumbers.php"
)

type ApiResponse struct {
	SEcho                interface{}     `json:"sEcho"`
	ITotalRecords        interface{}     `json:"iTotalRecords"`
	ITotalDisplayRecords interface{}     `json:"iTotalDisplayRecords"`
	AAData               [][]interface{} `json:"aaData"`
}

type Client struct {
	HTTPClient *http.Client
	Csstr      string
	Mutex      sync.Mutex
}

func NewClient() *Client {
	jar, _ := cookiejar.New(nil)
	return &Client{
		HTTPClient: &http.Client{
			Jar:     jar,
			Timeout: 60 * time.Second,
		},
	}
}

func (c *Client) ensureSession() error {
	if c.Csstr != "" {
		return nil
	}
	fmt.Println("[Masdar] Csstr token missing, Login start...")
	return c.performLogin()
}

func (c *Client) performLogin() error {
	// ... (Login Logic Same as previous, keeping it short here) ...
	req, _ := http.NewRequest("GET", LoginURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K)")
	resp, err := c.HTTPClient.Do(req)
	if err != nil { return err }
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(resp.Body)
	
	re := regexp.MustCompile(`What is (\d+) \+ (\d+) = \?`)
	matches := re.FindStringSubmatch(string(bodyBytes))
	if len(matches) < 3 { return errors.New("captcha failed") }
	n1, _ := strconv.Atoi(matches[1])
	n2, _ := strconv.Atoi(matches[2])
	
	data := url.Values{}
	data.Set("username", "Kami526")
	data.Set("password", "Kami526")
	data.Set("capt", strconv.Itoa(n1+n2))

	loginReq, _ := http.NewRequest("POST", SigninURL, bytes.NewBufferString(data.Encode()))
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginReq.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K)")
	loginReq.Header.Set("Referer", LoginURL)
	resp, err = c.HTTPClient.Do(loginReq)
	if err != nil { return err }
	defer resp.Body.Close()

	reportReq, _ := http.NewRequest("GET", ReportsPage, nil)
	reportReq.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K)")
	resp, err = c.HTTPClient.Do(reportReq)
	if err != nil { return err }
	defer resp.Body.Close()
	rBody, _ := io.ReadAll(resp.Body)
	
	csstrRe := regexp.MustCompile(`csstr=([^"&']+)`)
	match := csstrRe.FindStringSubmatch(string(rBody))
	if len(match) > 1 {
		c.Csstr = match[1]
	} else {
		fbRe := regexp.MustCompile(`["']csstr["']\s*[:=]\s*["']([^"']+)["']`)
		m2 := fbRe.FindStringSubmatch(string(rBody))
		if len(m2) > 1 { c.Csstr = m2[1] }
	}
	return nil
}

// ---------------------- SMS CLEANING (User Removed) ----------------------

func (c *Client) GetSMSLogs() ([]byte, error) {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()

	for i := 0; i < 2; i++ {
		if err := c.ensureSession(); err != nil { return nil, err }

		now := time.Now()
		startDate := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		
		params := url.Values{}
		params.Set("fdate1", startDate.Format("2006-01-02")+" 00:00:00")
		params.Set("fdate2", now.Format("2006-01-02")+" 23:59:59")
		params.Set("frange", "")
		params.Set("fclient", "")
		params.Set("fg", "0")
		if c.Csstr != "" { params.Set("csstr", c.Csstr) }
		params.Set("sEcho", "3")
		params.Set("iDisplayLength", "100") 
		params.Set("iSortingCols", "1")
		params.Set("sSortDir_0", "desc")

		req, _ := http.NewRequest("GET", SMSApiURL+"?"+params.Encode(), nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K)")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")

		resp, err := c.HTTPClient.Do(req)
		if err != nil { return nil, err }
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)

		if bytes.Contains(body, []byte("<!DOCTYPE html>")) {
			c.Csstr = ""
			c.HTTPClient.Jar, _ = cookiejar.New(nil)
			continue
		}

		return cleanMasdarSMS(body)
	}
	return nil, errors.New("failed after retry")
}

func cleanMasdarSMS(rawJSON []byte) ([]byte, error) {
	var apiResp ApiResponse
	if err := json.Unmarshal(rawJSON, &apiResp); err != nil { return rawJSON, nil }

	var cleanedRows [][]interface{}

	// Raw: [Date(0), Country(1), Number(2), Service(3), User(4), Message(5), Currency(6), Cost(7), Status(8)]
	// Target: [Date, Country, Number, Service, Message, Currency, Cost, Status] (User Removed)

	for _, row := range apiResp.AAData {
		if len(row) > 8 { // Ensure we have enough columns
			// Message Cleanup (Index 5 in RAW)
			msg, _ := row[5].(string)
			msg = html.UnescapeString(msg)
			msg = strings.ReplaceAll(msg, "null", "")

			newRow := []interface{}{
				row[0], // Date
				row[1], // Country
				row[2], // Number
				row[3], // Service
				// SKIPPING ROW[4] (User)
				msg,    // Message (Moved to position 4)
				row[6], // Currency
				row[7], // Cost
				row[8], // Status
			}
			cleanedRows = append(cleanedRows, newRow)
		}
	}
	
	apiResp.AAData = cleanedRows
	return json.Marshal(apiResp)
}

// ---------------------- NUMBERS CLEANING (Same as before) ----------------------

func (c *Client) GetNumberStats() ([]byte, error) {
	// ... (Same logic as previous turn) ...
	c.Mutex.Lock()
	defer c.Mutex.Unlock()

	for i := 0; i < 2; i++ {
		if err := c.ensureSession(); err != nil { return nil, err }

		params := url.Values{}
		params.Set("sEcho", "2")
		params.Set("iDisplayLength", "-1")
		params.Set("iSortingCols", "1")
		params.Set("sSortDir_0", "asc")
		if c.Csstr != "" { params.Set("csstr", c.Csstr) }

		req, _ := http.NewRequest("GET", NumberApiURL+"?"+params.Encode(), nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K)")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")

		resp, err := c.HTTPClient.Do(req)
		if err != nil { return nil, err }
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)

		if bytes.Contains(body, []byte("<!DOCTYPE html>")) {
			c.Csstr = ""
			c.HTTPClient.Jar, _ = cookiejar.New(nil)
			continue
		}
		return cleanMasdarNumbers(body)
	}
	return nil, errors.New("failed")
}

func cleanMasdarNumbers(rawJSON []byte) ([]byte, error) {
	// ... (Same cleaning logic as previous turn) ...
	var apiResp ApiResponse
	if err := json.Unmarshal(rawJSON, &apiResp); err != nil { return rawJSON, nil }

	var cleanedRows [][]interface{}
	rePrice := regexp.MustCompile(`[\d\.]+`)

	for _, row := range apiResp.AAData {
		if len(row) > 7 {
			rangeName := row[1]
			prefix := row[2]
			number := row[3]
			
			priceHTML, _ := row[4].(string)
			billingType := "Weekly"
			if strings.Contains(strings.ToLower(priceHTML), "monthly") { billingType = "Monthly" }

			currency := "$"
			if strings.Contains(priceHTML, "€") { currency = "€" }
			else if strings.Contains(priceHTML, "£") { currency = "£" }
			
			priceVal := "0"
			matches := rePrice.FindAllString(priceHTML, -1)
			if len(matches) > 0 { priceVal = matches[len(matches)-1] }
			priceStr := currency + " " + priceVal

			stats := row[7]

			newRow := []interface{}{
				rangeName,
				prefix,
				number,
				billingType,
				priceStr,
				stats,
			}
			cleanedRows = append(cleanedRows, newRow)
		}
	}
	apiResp.AAData = cleanedRows
	apiResp.ITotalRecords = len(cleanedRows)
	apiResp.ITotalDisplayRecords = len(cleanedRows)
	return json.Marshal(apiResp)
}
