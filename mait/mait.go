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
	Csstr      string // Session Key
	Mutex      sync.Mutex
}

var (
	activeClient *Client    
	clientMutex  sync.Mutex 
)

// GetSession: Returns existing client or creates new
func GetSession() *Client {
	clientMutex.Lock()
	defer clientMutex.Unlock()

	if activeClient != nil {
		return activeClient
	}

	jar, _ := cookiejar.New(nil)
	activeClient = &Client{
		HTTPClient: &http.Client{
			Jar:     jar,
			Timeout: 60 * time.Second,
		},
	}
	return activeClient
}

// ---------------------------------------------------------
// LOGIN LOGIC (Enhanced Headers to bypass 403)
// ---------------------------------------------------------

func (c *Client) ensureSession() error {
	if c.Csstr != "" {
		return nil
	}
	fmt.Println("[Masdar] Csstr token missing, Login start...")
	return c.performLogin()
}

func (c *Client) performLogin() error {
	fmt.Println("[Masdar] >> Step 1: Login Page")
	
	req, _ := http.NewRequest("GET", LoginURL, nil)
	// ==========================================================
	// FIX: Headers exactly matching your Browser Trace
	// ==========================================================
	req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Mobile Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9,ur-PK;q=0.8,ur;q=0.7")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Connection", "keep-alive")
	// Referer is not needed for the very first hit usually, but if blocked, we can add:
	// req.Header.Set("Referer", BaseURL + "/ints/agent/") 

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(resp.Body)
	bodyString := string(bodyBytes)

	// ==========================================================
	// CAPTCHA LOGIC WITH DEBUG DUMP
	// ==========================================================
	fmt.Println("[Masdar] >> Step 2: Solving Captcha")
	
	re := regexp.MustCompile(`What\s+is\s+(\d+)\s*\+\s*(\d+)`)
	matches := re.FindStringSubmatch(bodyString)
	
	if len(matches) < 3 {
		fmt.Println("\n\n================ [ DEBUG: HTML START (Login Page) ] ================")
		// صرف ٹائٹل اور باڈی کا کچھ حصہ پرنٹ کریں تاکہ لاگ بہت بڑا نہ ہو
		if len(bodyString) > 1000 {
			fmt.Println(bodyString[:1000] + "... (truncated)")
		} else {
			fmt.Println(bodyString)
		}
		fmt.Println("================ [ DEBUG: HTML END ] ================\n\n")
		return errors.New("captcha failed: Server likely returned 403 Forbidden or different HTML")
	}

	num1, _ := strconv.Atoi(matches[1])
	num2, _ := strconv.Atoi(matches[2])
	captchaAns := strconv.Itoa(num1 + num2)
	fmt.Printf("[Masdar] Captcha Solved: %s + %s = %s\n", matches[1], matches[2], captchaAns)

	// Step 3: Login POST
	data := url.Values{}
	data.Set("username", "Kami526") 
	data.Set("password", "Kami526") 
	data.Set("capt", captchaAns)

	loginReq, _ := http.NewRequest("POST", SigninURL, bytes.NewBufferString(data.Encode()))
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginReq.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Mobile Safari/537.36")
	loginReq.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8")
	loginReq.Header.Set("Accept-Language", "en-US,en;q=0.9,ur-PK;q=0.8,ur;q=0.7")
	loginReq.Header.Set("Upgrade-Insecure-Requests", "1")
	loginReq.Header.Set("Origin", BaseURL)
	loginReq.Header.Set("Referer", LoginURL)

	resp, err = c.HTTPClient.Do(loginReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Step 4: Get Csstr Token from Reports Page
	fmt.Println("[Masdar] >> Step 3: Getting Csstr Token")
	reportReq, _ := http.NewRequest("GET", ReportsPage, nil)
	reportReq.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Mobile Safari/537.36")
	reportReq.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8")
	reportReq.Header.Set("Upgrade-Insecure-Requests", "1")
	reportReq.Header.Set("Referer", BaseURL+"/ints/agent/MySMSNumbers") // Updated Referer based on trace

	resp, err = c.HTTPClient.Do(reportReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	reportBody, _ := io.ReadAll(resp.Body)
	reportString := string(reportBody)
	
	csstrRe := regexp.MustCompile(`csstr=([a-zA-Z0-9]+)`)
	csstrMatch := csstrRe.FindStringSubmatch(reportString)
	
	if len(csstrMatch) > 1 {
		c.Csstr = csstrMatch[1] 
		fmt.Println("[Masdar] SUCCESS: Found Csstr:", c.Csstr)
	} else {
		fallbackRe := regexp.MustCompile(`["']csstr["']\s*[:=]\s*["']?([^"']+)["']?`)
		match2 := fallbackRe.FindStringSubmatch(reportString)
		if len(match2) > 1 {
			c.Csstr = match2[1]
			fmt.Println("[Masdar] SUCCESS: Found Csstr (Fallback):", c.Csstr)
		} else {
			// If blocked here, print logs
			if strings.Contains(reportString, "Forbidden") {
				fmt.Println("[Masdar] Error: 403 Forbidden on Reports Page")
			}
			return errors.New("csstr token not found (Login likely failed)")
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
			if i == 0 {
				c.Csstr = ""
				c.HTTPClient.Jar, _ = cookiejar.New(nil)
				continue
			}
			return nil, err
		}

		now := time.Now()
		todayDate := now.Format("2006-01-02")
		fdate1 := todayDate + " 00:00:00"
		fdate2 := todayDate + " 23:59:59"

		params := url.Values{}
		params.Set("fdate1", fdate1)
		params.Set("fdate2", fdate2)
		params.Set("frange", "")
		params.Set("fclient", "")
		params.Set("fg", "0")
		if c.Csstr != "" {
			params.Set("csstr", c.Csstr)
		}
		params.Set("sEcho", "1")
		params.Set("iDisplayStart", "0")
		params.Set("iDisplayLength", "100") 
		params.Set("mDataProp_0", "0") 

		finalURL := SMSApiURL + "?" + params.Encode()

		req, _ := http.NewRequest("GET", finalURL, nil)
		// Headers matching trace
		req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Mobile Safari/537.36")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
		req.Header.Set("Referer", ReportsPage)
		req.Header.Set("Accept-Language", "en-US,en;q=0.9,ur-PK;q=0.8,ur;q=0.7")

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)

		if bytes.Contains(body, []byte("<!DOCTYPE html>")) || bytes.Contains(body, []byte("<html")) {
			// Check if it's 403 Forbidden
			if bytes.Contains(body, []byte("Forbidden")) {
				fmt.Println("[Masdar] Critical: 403 Forbidden on API Call. Check headers/IP.")
				// Don't retry immediately if IP is blocked, but we'll try re-login logic once
			} else {
				fmt.Println("[Masdar] Session Expired. Re-logging...")
			}
			c.Csstr = "" 
			c.HTTPClient.Jar, _ = cookiejar.New(nil)
			continue 
		}

		cleanedJSON, err := cleanMasdarSMS(body)
		if err != nil {
			if i == 0 { continue }
			return nil, err
		}
		return cleanedJSON, nil
	}
	return nil, errors.New("failed after retry (login loop)")
}

func cleanMasdarSMS(rawJSON []byte) ([]byte, error) {
	var apiResp ApiResponse
	if err := json.Unmarshal(rawJSON, &apiResp); err != nil {
		return rawJSON, nil
	}

	var cleanedRows [][]interface{}

	for _, row := range apiResp.AAData {
		if len(row) > 8 {
			msg, _ := row[5].(string)
			msg = html.UnescapeString(msg)
			msg = strings.ReplaceAll(msg, "null", "")

			newRow := []interface{}{
				row[0], // Date
				row[1], // Range
				row[2], // Number
				row[3], // CLI
				msg,    // SMS
				row[6], // Currency
				row[7], // My Payout
				row[8], // Client Payout
			}
			cleanedRows = append(cleanedRows, newRow)
		}
	}
	
	apiResp.AAData = cleanedRows
	return json.Marshal(apiResp)
}

// ---------------------- NUMBERS CLEANING ----------------------

func (c *Client) GetNumberStats() ([]byte, error) {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()

	for i := 0; i < 2; i++ {
		if err := c.ensureSession(); err != nil {
			if i == 0 {
				c.Csstr = ""
				c.HTTPClient.Jar, _ = cookiejar.New(nil)
				continue
			}
			return nil, err
		}

		params := url.Values{}
		params.Set("frange", "")
		params.Set("fclient", "")
		if c.Csstr != "" {
			params.Set("csstr", c.Csstr)
		}
		params.Set("sEcho", "2")
		params.Set("iDisplayStart", "0")
		params.Set("iDisplayLength", "-1") 
		params.Set("sSortDir_0", "asc")

		finalURL := NumberApiURL + "?" + params.Encode()

		req, _ := http.NewRequest("GET", finalURL, nil)
		// Exact headers
		req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Mobile Safari/537.36")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
		req.Header.Set("Referer", BaseURL+"/ints/agent/MySMSNumbers")
		req.Header.Set("Accept-Language", "en-US,en;q=0.9,ur-PK;q=0.8,ur;q=0.7")

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)

		if bytes.Contains(body, []byte("<!DOCTYPE html>")) || bytes.Contains(body, []byte("<html")) {
			fmt.Println("[Masdar] Session/IP Issue on Numbers, Retrying...")
			c.Csstr = ""
			c.HTTPClient.Jar, _ = cookiejar.New(nil)
			continue
		}

		cleanedJSON, err := cleanMasdarNumbers(body)
		if err != nil {
			if i == 0 { continue }
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
		if len(row) > 7 {
			rangeName := row[1]
			prefix := row[2]
			number := row[3]
			
			priceHTML, _ := row[4].(string)
			billingType := "Weekly"
			if strings.Contains(strings.ToLower(priceHTML), "monthly") {
				billingType = "Monthly"
			}

			currency := "$"
			if strings.Contains(priceHTML, "€") {
				currency = "€"
			} else if strings.Contains(priceHTML, "£") {
				currency = "£"
			}
			
			priceVal := "0"
			matches := rePrice.FindAllString(priceHTML, -1)
			if len(matches) > 0 {
				priceVal = matches[len(matches)-1]
			}
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