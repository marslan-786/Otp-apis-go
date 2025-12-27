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

// URLs for Masdar Alkon (IP: 217.182.195.194)
const (
	BaseURL      = "http://217.182.195.194"
	LoginURL     = BaseURL + "/ints/login"
	SigninURL    = BaseURL + "/ints/signin"
	ReportsPage  = BaseURL + "/ints/agent/SMSCDRReports"
	SMSApiURL    = BaseURL + "/ints/agent/res/data_smscdr.php"
	NumberApiURL = BaseURL + "/ints/agent/res/data_smsnumbers.php"
)

// Response Wrapper
type ApiResponse struct {
	SEcho                interface{}     `json:"sEcho"`
	ITotalRecords        interface{}     `json:"iTotalRecords"`
	ITotalDisplayRecords interface{}     `json:"iTotalDisplayRecords"`
	AAData               [][]interface{} `json:"aaData"`
}

type Client struct {
	HTTPClient *http.Client
	SessKey    string
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
	if c.SessKey != "" {
		return nil
	}
	fmt.Println("[Masdar] Session Key missing, Login start...")
	return c.performLogin()
}

func (c *Client) performLogin() error {
	fmt.Println("[Masdar] >> Step 1: Login Page")
	
	req, _ := http.NewRequest("GET", LoginURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K)")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(resp.Body)
	bodyString := string(bodyBytes)

	// Captcha Logic: What is 5 + 2 = ?
	fmt.Println("[Masdar] >> Step 2: Solving Captcha")
	re := regexp.MustCompile(`What is (\d+) \+ (\d+) = \?`)
	matches := re.FindStringSubmatch(bodyString)
	if len(matches) < 3 {
		return errors.New("captcha math failed")
	}
	num1, _ := strconv.Atoi(matches[1])
	num2, _ := strconv.Atoi(matches[2])
	captchaAns := strconv.Itoa(num1 + num2)
	fmt.Printf("[Masdar] Captcha Solved: %s\n", captchaAns)

	// Step 3: Login POST
	data := url.Values{}
	data.Set("username", "Kami526")
	data.Set("password", "Kami526")
	data.Set("capt", captchaAns)

	loginReq, _ := http.NewRequest("POST", SigninURL, bytes.NewBufferString(data.Encode()))
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginReq.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K)")
	loginReq.Header.Set("Referer", LoginURL)

	resp, err = c.HTTPClient.Do(loginReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Step 4: Get SessKey
	fmt.Println("[Masdar] >> Step 3: Getting SessKey")
	reportReq, _ := http.NewRequest("GET", ReportsPage, nil)
	reportReq.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K)")

	resp, err = c.HTTPClient.Do(reportReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	reportBody, _ := io.ReadAll(resp.Body)
	
	// Unlike Neon, Masdar seems to use 'API Token' in HTML or sesskey in URL.
	// Based on your previous logs: sesskey=XH-MekZQVTxCT09ERA== (Wait, that looks like API Token format)
	// But in your request log: sesskey=5h540dk2drf3gfl4npdf94hr4c (This is PHPSESSID in cookie, odd)
	// Let's rely on Cookie for Auth, but if URL needs token we extract "API Token" from HTML sidebar.
	
	// Trying to extract API Token from Sidebar HTML: "API Token : XH-MekZQVTxCT09ERA=="
	tokenRe := regexp.MustCompile(`API Token : ([a-zA-Z0-9\-\_\=\+]+)`)
	tokenMatch := tokenRe.FindStringSubmatch(string(reportBody))
	
	if len(tokenMatch) > 1 {
		c.SessKey = tokenMatch[1]
		fmt.Println("[Masdar] API Token Found:", c.SessKey)
	} else {
		// Fallback: Sometimes sesskey is just not needed if cookie is there, 
		// but let's try standard sesskey regex too just in case.
		sessRe := regexp.MustCompile(`sesskey=([a-zA-Z0-9%=]+)`)
		sessMatch := sessRe.FindStringSubmatch(string(reportBody))
		if len(sessMatch) > 1 {
			c.SessKey = sessMatch[1]
		} else {
			fmt.Println("[Masdar] Warning: Token not found, proceeding with Cookie only.")
		}
	}

	return nil
}

// ---------------------- SMS CLEANING ----------------------

func (c *Client) GetSMSLogs() ([]byte, error) {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()

	for i := 0; i < 2; i++ {
		if err := c.ensureSession(); err != nil {
			return nil, err
		}

		now := time.Now()
		// Fixed Start Date Logic (1st of Month)
		startDate := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		fdate1 := startDate.Format("2006-01-02") + " 00:00:00"
		fdate2 := now.Format("2006-01-02") + " 23:59:59"

		params := url.Values{}
		params.Set("fdate1", fdate1)
		params.Set("fdate2", fdate2)
		params.Set("frange", "")
		params.Set("fclient", "")
		params.Set("fg", "0")
		// Masdar might use 'csstr' token based on logs, but usually cookies work. 
		// We will try without first.
		params.Set("sEcho", "3")
		params.Set("iDisplayLength", "100") 
		params.Set("iSortingCols", "1")
		params.Set("sSortDir_0", "desc")

		finalURL := SMSApiURL + "?" + params.Encode()
		fmt.Println("[Masdar] Fetching SMS...")

		req, _ := http.NewRequest("GET", finalURL, nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K)")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)

		if bytes.Contains(body, []byte("<!DOCTYPE html>")) {
			c.SessKey = "" // Reset
			c.HTTPClient.Jar, _ = cookiejar.New(nil)
			continue
		}

		// Clean Data
		cleanedJSON, err := cleanMasdarSMS(body)
		if err != nil {
			return nil, err
		}
		return cleanedJSON, nil
	}
	return nil, errors.New("failed after retry")
}

func cleanMasdarSMS(rawJSON []byte) ([]byte, error) {
	var apiResp ApiResponse
	if err := json.Unmarshal(rawJSON, &apiResp); err != nil {
		return rawJSON, nil
	}

	for i, row := range apiResp.AAData {
		if len(row) > 5 {
			msg, ok := row[5].(string)
			if ok {
				cleanMsg := html.UnescapeString(msg)
				// Remove specific Masdar junk if any
				apiResp.AAData[i][5] = cleanMsg
			}
		}
	}
	return json.Marshal(apiResp)
}

// ---------------------- NUMBERS CLEANING ----------------------

func (c *Client) GetNumberStats() ([]byte, error) {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()

	for i := 0; i < 2; i++ {
		if err := c.ensureSession(); err != nil {
			return nil, err
		}

		params := url.Values{}
		params.Set("frange", "")
		params.Set("fclient", "")
		params.Set("sEcho", "2")
		params.Set("iDisplayLength", "-1") // Fetch All
		params.Set("iSortingCols", "1")
		params.Set("sSortDir_0", "asc")

		finalURL := NumberApiURL + "?" + params.Encode()
		fmt.Println("[Masdar] Fetching Numbers...")

		req, _ := http.NewRequest("GET", finalURL, nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K)")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)

		if bytes.Contains(body, []byte("<!DOCTYPE html>")) {
			c.SessKey = ""
			c.HTTPClient.Jar, _ = cookiejar.New(nil)
			continue
		}

		cleanedJSON, err := cleanMasdarNumbers(body)
		if err != nil {
			return nil, err
		}
		return cleanedJSON, nil
	}
	return nil, errors.New("failed after retry")
}

func cleanMasdarNumbers(rawJSON []byte) ([]byte, error) {
	var apiResp ApiResponse
	if err := json.Unmarshal(rawJSON, &apiResp); err != nil {
		return rawJSON, nil
	}

	var cleanedRows [][]interface{}
	rePrice := regexp.MustCompile(`[\d\.]+`)

	for _, row := range apiResp.AAData {
		// Raw: [Checkbox, RangeName, Prefix, Number, PriceHTML, Action, Empty, Stats]
		if len(row) > 4 {
			number := row[3] // Index 3 is Number
			
			rawPrice, _ := row[4].(string) // Index 4 has Price HTML
			
			currency := "$"
			if strings.Contains(rawPrice, "€") {
				currency = "€"
			} else if strings.Contains(rawPrice, "£") {
				currency = "£"
			}

			price := 0.0
			priceMatches := rePrice.FindAllString(rawPrice, -1)
			if len(priceMatches) > 0 {
				lastVal := priceMatches[len(priceMatches)-1]
				p, _ := strconv.ParseFloat(lastVal, 64)
				price = p
			}

			// Clean Format: [Number, Count(1), Currency, Price, 0]
			newRow := []interface{}{
				number,
				"1",
				currency,
				price,
				0,
			}
			cleanedRows = append(cleanedRows, newRow)
		}
	}

	apiResp.AAData = cleanedRows
	apiResp.ITotalRecords = len(cleanedRows)
	apiResp.ITotalDisplayRecords = len(cleanedRows)

	return json.Marshal(apiResp)
}
